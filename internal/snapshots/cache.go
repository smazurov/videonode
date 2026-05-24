// Package snapshots serves source/composer snapshots over HTTP, backed by
// an in-memory JPEG cache. The cache coalesces concurrent refreshes so
// multiple viewers of the same entity trigger at most one upstream RPC
// per refresh interval. Cached entries carry the producer's frame_idx so
// HTTP ETag negotiation can return 304 when nothing has advanced.
//
// Two delivery surfaces sit on top:
//   - GET /api/{kind}/{id}/snapshot.jpg   — one-shot JPEG
//   - GET /api/{kind}/{id}/preview.mjpg   — multipart/x-mixed-replace stream
//
// The preview stream registers a Subscriber; the scheduler goroutine
// refreshes the cache at the subscriber's chosen fps (clamped to the
// configured cap) and broadcasts new entries to every subscriber.
package snapshots

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Kind identifies the entity family being snapshotted.
type Kind string

// Snapshot entity kinds. Each maps to one URL prefix.
const (
	KindSource   Kind = "sources"
	KindComposer Kind = "composers"
)

// Format describes the raw pixel format returned by the upstream Fetcher.
type Format int

// Raw pixel formats handed by upstream sources/composers.
const (
	FormatNV12 Format = iota
	FormatBGRA
)

// Frame is the upstream-fetched raw frame plus metadata.
type Frame struct {
	Bytes      []byte
	Format     Format
	Width      int
	Height     int
	FrameIdx   uint64
	CapturedNs uint64
}

// Entry is the cached JPEG-encoded snapshot served to HTTP clients.
type Entry struct {
	JPEG       []byte
	FrameIdx   uint64
	Width      int
	Height     int
	CapturedAt time.Time
}

// Fetcher dials the upstream RPC and returns a raw frame. Implemented by
// internal/streams/pipeline.Pipeline.
type Fetcher interface {
	SnapshotSource(ctx context.Context, id string) (Frame, error)
	SnapshotComposer(ctx context.Context, id string) (Frame, error)
}

// Encoder converts a raw Frame's pixels to JPEG bytes. Default impl pipes
// through ffmpeg; tests override.
type Encoder interface {
	EncodeJPEG(frame Frame) ([]byte, error)
}

// ErrNoFrame is returned when the upstream has not produced any frame yet.
var ErrNoFrame = errors.New("snapshots: no frame produced yet")

// ErrNotFound is returned when the requested entity does not exist.
var ErrNotFound = errors.New("snapshots: entity not found")

// Config tunes cache + scheduler behavior.
type Config struct {
	// StaleAfter is the upper bound on how old a cached entry can be
	// before a Get triggers a refresh. Defaults to 800ms.
	StaleAfter time.Duration
	// MaxFPS clamps the per-stream fps requested via ?fps=N. Default 10.
	MaxFPS int
	// DefaultFPS is the preview rate when ?fps is omitted. Default 1.
	DefaultFPS int
	// RPCTimeout bounds the upstream Snapshot RPC. Default 3s.
	RPCTimeout time.Duration
}

func (c *Config) fillDefaults() {
	if c.StaleAfter <= 0 {
		c.StaleAfter = 800 * time.Millisecond
	}
	if c.MaxFPS <= 0 {
		c.MaxFPS = 10
	}
	if c.DefaultFPS <= 0 {
		c.DefaultFPS = 1
	}
	if c.RPCTimeout <= 0 {
		c.RPCTimeout = 3 * time.Second
	}
}

// ClampFPS returns the requested fps clamped to [1, cfg.MaxFPS]. If
// requested <= 0, DefaultFPS is used.
func (c Config) ClampFPS(requested int) int {
	if requested <= 0 {
		requested = c.DefaultFPS
	}
	if requested < 1 {
		requested = 1
	}
	if requested > c.MaxFPS {
		requested = c.MaxFPS
	}
	return requested
}

// Cache is the snapshot store + refresh coordinator. It is safe to call
// any public method from multiple goroutines concurrently.
type Cache struct {
	cfg     Config
	fetcher Fetcher
	encoder Encoder

	mu       sync.Mutex
	entries  map[string]*Entry         // key: cacheKey(kind, id)
	inflight map[string]chan struct{}  // signals when an in-flight refresh completes
	subs     map[string]*subscriberSet // active subscriber sets for preview streams
}

// NewCache returns a Cache wired to the given Fetcher + Encoder.
func NewCache(cfg Config, fetcher Fetcher, encoder Encoder) *Cache {
	cfg.fillDefaults()
	return &Cache{
		cfg:      cfg,
		fetcher:  fetcher,
		encoder:  encoder,
		entries:  make(map[string]*Entry),
		inflight: make(map[string]chan struct{}),
		subs:     make(map[string]*subscriberSet),
	}
}

// Config exposes the active configuration (after defaults). The preview
// handler reads MaxFPS and DefaultFPS through this accessor.
func (c *Cache) Config() Config { return c.cfg }

func cacheKey(kind Kind, id string) string { return string(kind) + ":" + id }

// Get returns the latest JPEG entry for {kind, id}, refreshing from the
// upstream Fetcher if the cached copy is older than StaleAfter (or absent).
// Concurrent Get calls for the same key share a single in-flight refresh.
func (c *Cache) Get(ctx context.Context, kind Kind, id string) (*Entry, error) {
	key := cacheKey(kind, id)

	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		if time.Since(e.CapturedAt) < c.cfg.StaleAfter {
			c.mu.Unlock()
			return e, nil
		}
	}
	c.mu.Unlock()
	return c.refreshCoalesced(ctx, kind, id)
}

// Refresh forces an upstream fetch even if the cached entry is fresh.
// Used by the preview scheduler so it can drive higher fps than 1/StaleAfter.
// Concurrent Refresh + Get calls still share one in-flight RPC.
func (c *Cache) Refresh(ctx context.Context, kind Kind, id string) (*Entry, error) {
	return c.refreshCoalesced(ctx, kind, id)
}

// refreshCoalesced is the shared "fetch from upstream, store, broadcast"
// helper. Concurrent callers for the same key wait on a single in-flight
// channel so we never run two upstream RPCs at the same time.
func (c *Cache) refreshCoalesced(ctx context.Context, kind Kind, id string) (*Entry, error) {
	key := cacheKey(kind, id)
	c.mu.Lock()
	if ch, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.mu.Lock()
		e, found := c.entries[key]
		c.mu.Unlock()
		if !found {
			return nil, ErrNoFrame
		}
		return e, nil
	}
	ch := make(chan struct{})
	c.inflight[key] = ch
	c.mu.Unlock()

	entry, err := c.refresh(ctx, kind, id)

	c.mu.Lock()
	delete(c.inflight, key)
	if entry != nil {
		c.entries[key] = entry
	}
	c.mu.Unlock()
	close(ch)

	if err != nil {
		return nil, err
	}
	return entry, nil
}

// refresh always goes to the upstream — no cache check. Returns the
// fresh Entry (caller stores it).
func (c *Cache) refresh(ctx context.Context, kind Kind, id string) (*Entry, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, c.cfg.RPCTimeout)
	defer cancel()

	var (
		frame Frame
		err   error
	)
	switch kind {
	case KindSource:
		frame, err = c.fetcher.SnapshotSource(rpcCtx, id)
	case KindComposer:
		frame, err = c.fetcher.SnapshotComposer(rpcCtx, id)
	default:
		return nil, fmt.Errorf("snapshots: unknown kind %q", kind)
	}
	if err != nil {
		return nil, err
	}
	if len(frame.Bytes) == 0 || frame.Width == 0 || frame.Height == 0 {
		return nil, ErrNoFrame
	}

	jpeg, err := c.encoder.EncodeJPEG(frame)
	if err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	return &Entry{
		JPEG:       jpeg,
		FrameIdx:   frame.FrameIdx,
		Width:      frame.Width,
		Height:     frame.Height,
		CapturedAt: time.Now(),
	}, nil
}
