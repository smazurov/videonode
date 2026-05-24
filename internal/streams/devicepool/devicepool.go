// Package devicepool owns the lifecycle of persistent videonode-source
// processes. The daemon spawns one source per configured device_id at
// startup and keeps it alive across stream/encoder/composer churn and
// device plug/unplug events. The source's data plane (SCM_RIGHTS socket)
// stays stable; encoders/composers connect to a long-lived endpoint.
//
// The pool collaborates with the pipeline package: it calls
// Pipeline.EnsurePersistentSource (which spawns + pins) and
// Pipeline.StopPersistentSource (which unpins + stops). Pinning excludes
// the device from the ProducerRegistry's ToStart/ToStop deltas so
// encoder churn never touches it.
//
// Device identity flows through gRPC. After spawn, the daemon resolves
// device_id → /dev/videoN via the supplied PathResolver and calls
// pipelinectl.Manager.SendSetDevice(device_id, path). On hotplug
// add/remove, OnDeviceEvent forwards new state. Empty path = detach,
// source goes back to placeholder broadcast.
//
// Crash policy: today, dead sources stay dead. process.Pool surfaces the
// exit via its OnStateChange callback (wired by the pipeline), and the
// pool logs at error level. A future restart policy hook lands here.
package devicepool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// PinnedSource is the subset of the pipeline.Pipeline API the device
// pool needs. Defined as an interface for testability and to avoid an
// import cycle with internal/streams.
type PinnedSource interface {
	EnsurePersistentSource(deviceID string) error
	StopPersistentSource(deviceID string)
}

// DeviceSetter is the subset of pipelinectl.Manager the pool uses to
// push device-path updates to a source's gRPC server.
type DeviceSetter interface {
	SendSetDevice(ctx context.Context, deviceID, devicePath string) error
}

// PathResolver maps a device_id to its current /dev/videoN path, or ""
// when the device is not present.
type PathResolver func(deviceID string) string

// DeviceLister yields the set of device_ids the pool should manage.
// Today this returns the union of `Device` fields across configured
// streams; tomorrow it can read a dedicated `[[devices]]` section.
type DeviceLister func() []string

// Config bundles the pool's dependencies. Pipeline and Setter are
// required; Lister and Resolver default to no-op if nil but render the
// pool useless.
type Config struct {
	Pipeline PinnedSource
	Setter   DeviceSetter
	Resolver PathResolver
	Lister   DeviceLister
	Logger   logging.Logger
	// SetDeviceTimeout bounds each SetDevice gRPC call. Defaults to 3s.
	SetDeviceTimeout time.Duration
	// DialRetry controls how long Start waits for the source's gRPC UDS
	// to become reachable before giving up on the initial SetDevice.
	// Defaults to 10s with 200ms polling.
	DialRetryBudget   time.Duration
	DialRetryInterval time.Duration
}

// Pool owns persistent device sources.
type Pool struct {
	cfg    Config
	logger logging.Logger

	mu       sync.Mutex
	managed  map[string]struct{} // device_id → tracked
	lastPath map[string]string   // device_id → last path we pushed
}

// New constructs a Pool. Does not spawn anything until Start is called.
func New(cfg Config) *Pool {
	if cfg.Logger == nil {
		cfg.Logger = logging.GetLogger("devicepool")
	}
	if cfg.SetDeviceTimeout == 0 {
		cfg.SetDeviceTimeout = 3 * time.Second
	}
	if cfg.DialRetryBudget == 0 {
		cfg.DialRetryBudget = 10 * time.Second
	}
	if cfg.DialRetryInterval == 0 {
		cfg.DialRetryInterval = 200 * time.Millisecond
	}
	return &Pool{
		cfg:      cfg,
		logger:   cfg.Logger,
		managed:  make(map[string]struct{}),
		lastPath: make(map[string]string),
	}
}

// Start enumerates the configured device_ids, spawns a persistent source
// for each, and asynchronously resolves the current /dev path and pushes
// it via SetDevice. Safe to call once at daemon startup.
//
// Returns the first spawn error encountered; later devices in the list
// are still attempted. Spawn failures don't abort the pool — surface
// them for diagnostics while the rest of the daemon comes up.
func (p *Pool) Start(ctx context.Context) error {
	if p.cfg.Pipeline == nil {
		return fmt.Errorf("devicepool: Pipeline is required")
	}
	if p.cfg.Lister == nil {
		p.logger.Warn("devicepool: no DeviceLister configured; nothing to start")
		return nil
	}
	deviceIDs := p.cfg.Lister()
	if len(deviceIDs) == 0 {
		p.logger.Info("devicepool: no devices configured")
		return nil
	}

	var firstErr error
	seen := make(map[string]struct{}, len(deviceIDs))
	for _, id := range deviceIDs {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		if err := p.cfg.Pipeline.EnsurePersistentSource(id); err != nil {
			p.logger.Error("devicepool: spawn failed", "device_id", id, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		p.mu.Lock()
		p.managed[id] = struct{}{}
		p.mu.Unlock()
		p.logger.Info("devicepool: persistent source spawned", "device_id", id)

		// Async path resolve + SetDevice. Holds no locks across the
		// SetDevice round-trip; per-device retries are independent.
		go p.initialAssign(ctx, id)
	}
	return firstErr
}

// initialAssign waits up to DialRetryBudget for the source's gRPC UDS
// to be reachable (the daemon-side pipelinectl.Manager registers the
// connection once Describe succeeds), then pushes the current path. If
// no path is resolvable now, no SetDevice is sent — the source stays in
// placeholder mode until OnDeviceEvent fires on hotplug.
func (p *Pool) initialAssign(ctx context.Context, deviceID string) {
	resolver := p.cfg.Resolver
	if resolver == nil {
		return
	}
	deadline := time.Now().Add(p.cfg.DialRetryBudget)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}
		path := resolver(deviceID)
		if path == "" {
			// Device not present; nothing to push yet. Hotplug will call
			// OnDeviceEvent if/when it appears.
			return
		}
		if err := p.setDevice(ctx, deviceID, path); err != nil {
			// Likely the source's gRPC UDS isn't reachable yet; retry.
			time.Sleep(p.cfg.DialRetryInterval)
			continue
		}
		p.logger.Info("devicepool: initial device assigned",
			"device_id", deviceID, "path", path)
		return
	}
	p.logger.Warn("devicepool: initial SetDevice gave up",
		"device_id", deviceID, "budget", p.cfg.DialRetryBudget)
}

// Stop unpins and shuts down every managed source. Called from the
// daemon's shutdown sequence before process.Pool.StopAll.
func (p *Pool) Stop() error {
	if p.cfg.Pipeline == nil {
		return nil
	}
	p.mu.Lock()
	ids := make([]string, 0, len(p.managed))
	for id := range p.managed {
		ids = append(ids, id)
	}
	p.managed = make(map[string]struct{})
	p.lastPath = make(map[string]string)
	p.mu.Unlock()

	for _, id := range ids {
		p.cfg.Pipeline.StopPersistentSource(id)
	}
	return nil
}

// OnDeviceEvent is invoked by the device-detector/service_events layer
// when a configured device's presence flips. The devicePath argument is
// the current /dev/videoN string for ADD, or empty for REMOVE.
//
// No-op if the device_id isn't managed by the pool (e.g., hotplug
// detected a device that no configured stream consumes).
func (p *Pool) OnDeviceEvent(deviceID, devicePath string) {
	if deviceID == "" {
		return
	}
	p.mu.Lock()
	_, managed := p.managed[deviceID]
	if !managed {
		p.mu.Unlock()
		return
	}
	prev := p.lastPath[deviceID]
	p.mu.Unlock()
	if prev == devicePath {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.SetDeviceTimeout)
	defer cancel()
	if err := p.setDevice(ctx, deviceID, devicePath); err != nil {
		p.logger.Warn("devicepool: SetDevice on hotplug failed",
			"device_id", deviceID, "path", devicePath, "error", err)
		return
	}
	if devicePath == "" {
		p.logger.Info("devicepool: device detached", "device_id", deviceID)
	} else {
		p.logger.Info("devicepool: device attached",
			"device_id", deviceID, "path", devicePath)
	}
}

// setDevice issues the SetDevice gRPC call and remembers the path on
// success. Caller holds no locks.
func (p *Pool) setDevice(ctx context.Context, deviceID, devicePath string) error {
	if p.cfg.Setter == nil {
		return fmt.Errorf("devicepool: no DeviceSetter configured")
	}
	callCtx, cancel := context.WithTimeout(ctx, p.cfg.SetDeviceTimeout)
	defer cancel()
	if err := p.cfg.Setter.SendSetDevice(callCtx, deviceID, devicePath); err != nil {
		return err
	}
	p.mu.Lock()
	p.lastPath[deviceID] = devicePath
	p.mu.Unlock()
	return nil
}

// Managed reports whether device_id is currently in the pool. Used by
// the service-events layer to decide whether to forward a hotplug event.
func (p *Pool) Managed(deviceID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.managed[deviceID]
	return ok
}

// TODO(restart-policy): when process.Pool surfaces a managed source's
// unexpected exit, the pool should either respawn (bounded retry) or
// disable the device until an operator action restores it. Today the
// pool registers no OnStateChange listener; the source's pool entry
// stays in StateError until daemon restart.
