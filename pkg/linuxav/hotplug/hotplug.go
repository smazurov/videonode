//go:build linux

// Package hotplug provides pure Go device hotplug monitoring using netlink.
//
// This package monitors kernel device events without cgo by directly listening
// to netlinkKobjectUEvent messages from the kernel.
package hotplug

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"syscall"
)

// Action constants for device events.
const (
	ActionAdd     = "add"
	ActionRemove  = "remove"
	ActionChange  = "change"
	ActionMove    = "move"
	ActionBind    = "bind"
	ActionUnbind  = "unbind"
	ActionOnline  = "online"
	ActionOffline = "offline"
)

// Common subsystem names.
const (
	SubsystemVideo4Linux = "video4linux"
	SubsystemUSB         = "usb"
	SubsystemSound       = "sound"
	SubsystemBlock       = "block"
	SubsystemNet         = "net"
)

// Event represents a kernel device event.
type Event struct {
	Action    string            // "add", "remove", "change", etc.
	KObj      string            // Kernel object path: /devices/pci0000:00/...
	Subsystem string            // "video4linux", "usb", "sound", etc.
	DevType   string            // Device type if available
	DevName   string            // Device name (e.g., "video0")
	DevPath   string            // Device path (e.g., "/dev/video0")
	Env       map[string]string // All environment variables from the event
}

// Monitor listens for kernel device events via netlink.
type Monitor struct {
	fd        int
	filters   map[string]struct{}
	filtersMu sync.RWMutex
}

// netlinkKobjectUEvent is the netlink protocol for kernel object events.
const netlinkKobjectUEvent = 15

// NewMonitor creates a new device event monitor.
func NewMonitor() (*Monitor, error) {
	// Create netlink socket
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, netlinkKobjectUEvent)
	if err != nil {
		return nil, err
	}

	// Bind to the kernel broadcast group
	addr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: 1, // Kernel broadcast group
	}

	if err := syscall.Bind(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, err
	}

	return &Monitor{
		fd:      fd,
		filters: make(map[string]struct{}),
	}, nil
}

// AddSubsystemFilter adds a subsystem filter. Only events from matching
// subsystems will be returned. If no filters are added, all events pass through.
// This method is safe for concurrent use.
func (m *Monitor) AddSubsystemFilter(subsystem string) {
	m.filtersMu.Lock()
	m.filters[subsystem] = struct{}{}
	m.filtersMu.Unlock()
}

// Close releases the monitor resources.
func (m *Monitor) Close() error {
	return syscall.Close(m.fd)
}

// Run starts the monitor and sends events to the provided channel.
// It blocks until the context is cancelled or an error occurs.
// The events channel is closed when Run returns.
func (m *Monitor) Run(ctx context.Context, events chan<- Event) error {
	defer close(events)

	buf := make([]byte, 8192)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Set read timeout so we can check context periodically
		tv := syscall.Timeval{Sec: 1}
		if err := syscall.SetsockoptTimeval(m.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
			return err
		}

		n, _, err := syscall.Recvfrom(m.fd, buf, 0)
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				continue // Timeout, check context and retry
			}
			if errors.Is(err, syscall.EINTR) {
				continue // Interrupted, retry
			}
			return err
		}

		if n == 0 {
			continue
		}

		event := ParseUEvent(buf[:n])
		if event == nil {
			continue
		}

		// Apply subsystem filter
		m.filtersMu.RLock()
		filterCount := len(m.filters)
		_, matchesFilter := m.filters[event.Subsystem]
		m.filtersMu.RUnlock()
		if filterCount > 0 && !matchesFilter {
			continue
		}

		select {
		case events <- *event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ParseUEvent parses a kernel uevent message.
// Format: "ACTION@KOBJ\0KEY=VALUE\0KEY=VALUE\0..."
// Exported for testing.
func ParseUEvent(data []byte) *Event {
	if len(data) == 0 {
		return nil
	}

	// Skip libudev header if present (starts with "libudev")
	// libudev adds a binary header before the actual uevent
	if bytes.HasPrefix(data, []byte("libudev")) {
		// Skip past the header - find where the actual uevent starts
		// The header is followed by the uevent which starts with action@path
		for i := 0; i < len(data)-1; i++ {
			if data[i] == 0 {
				rest := data[i+1:]
				// Look for action@path pattern
				if idx := bytes.IndexByte(rest, '@'); idx > 0 && idx < 20 {
					data = rest
					break
				}
			}
		}
	}

	// Split by null bytes
	parts := bytes.Split(data, []byte{0})
	if len(parts) < 1 || len(parts[0]) == 0 {
		return nil
	}

	// First part is "ACTION@KOBJ"
	header := string(parts[0])
	atIdx := strings.Index(header, "@")
	if atIdx < 1 {
		return nil
	}

	event := &Event{
		Action: header[:atIdx],
		KObj:   header[atIdx+1:],
		Env:    make(map[string]string),
	}

	// Parse KEY=VALUE pairs
	for _, part := range parts[1:] {
		if len(part) == 0 {
			continue
		}

		kv := string(part)
		eqIdx := strings.Index(kv, "=")
		if eqIdx < 1 {
			continue
		}

		key := kv[:eqIdx]
		value := kv[eqIdx+1:]
		event.Env[key] = value

		// Extract common fields
		switch key {
		case "SUBSYSTEM":
			event.Subsystem = value
		case "DEVTYPE":
			event.DevType = value
		case "DEVNAME":
			event.DevName = value
		case "DEVPATH":
			event.DevPath = value
		}
	}

	return event
}
