//go:build linux

package hotplug

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestParseUEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected *Event
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: nil,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "no @ separator",
			input:    []byte("invalid"),
			expected: nil,
		},
		{
			name:     "missing action",
			input:    []byte("@/devices/foo"),
			expected: nil,
		},
		{
			name:  "simple add event",
			input: []byte("add@/devices/pci0000:00/video0\x00SUBSYSTEM=video4linux\x00DEVNAME=video0\x00"),
			expected: &Event{
				Action:    "add",
				KObj:      "/devices/pci0000:00/video0",
				Subsystem: "video4linux",
				DevName:   "video0",
				Env: map[string]string{
					"SUBSYSTEM": "video4linux",
					"DEVNAME":   "video0",
				},
			},
		},
		{
			name:  "remove event with multiple properties",
			input: []byte("remove@/devices/usb/1-1\x00SUBSYSTEM=usb\x00DEVTYPE=usb_device\x00DEVPATH=/devices/usb/1-1\x00PRODUCT=1234/5678/0100\x00"),
			expected: &Event{
				Action:    "remove",
				KObj:      "/devices/usb/1-1",
				Subsystem: "usb",
				DevType:   "usb_device",
				DevPath:   "/devices/usb/1-1",
				Env: map[string]string{
					"SUBSYSTEM": "usb",
					"DEVTYPE":   "usb_device",
					"DEVPATH":   "/devices/usb/1-1",
					"PRODUCT":   "1234/5678/0100",
				},
			},
		},
		{
			name:  "change event",
			input: []byte("change@/devices/sound/card0\x00SUBSYSTEM=sound\x00"),
			expected: &Event{
				Action:    "change",
				KObj:      "/devices/sound/card0",
				Subsystem: "sound",
				Env: map[string]string{
					"SUBSYSTEM": "sound",
				},
			},
		},
		{
			name:  "event with empty values",
			input: []byte("add@/devices/test\x00KEY1=value1\x00KEY2=\x00KEY3=value3\x00"),
			expected: &Event{
				Action: "add",
				KObj:   "/devices/test",
				Env: map[string]string{
					"KEY1": "value1",
					"KEY2": "",
					"KEY3": "value3",
				},
			},
		},
		{
			name:  "event with trailing nulls",
			input: []byte("bind@/devices/foo\x00SUBSYSTEM=pci\x00\x00\x00"),
			expected: &Event{
				Action:    "bind",
				KObj:      "/devices/foo",
				Subsystem: "pci",
				Env: map[string]string{
					"SUBSYSTEM": "pci",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseUEvent(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("expected %+v, got nil", tt.expected)
			}

			if result.Action != tt.expected.Action {
				t.Errorf("Action: expected %q, got %q", tt.expected.Action, result.Action)
			}
			if result.KObj != tt.expected.KObj {
				t.Errorf("KObj: expected %q, got %q", tt.expected.KObj, result.KObj)
			}
			if result.Subsystem != tt.expected.Subsystem {
				t.Errorf("Subsystem: expected %q, got %q", tt.expected.Subsystem, result.Subsystem)
			}
			if result.DevType != tt.expected.DevType {
				t.Errorf("DevType: expected %q, got %q", tt.expected.DevType, result.DevType)
			}
			if result.DevName != tt.expected.DevName {
				t.Errorf("DevName: expected %q, got %q", tt.expected.DevName, result.DevName)
			}
			if result.DevPath != tt.expected.DevPath {
				t.Errorf("DevPath: expected %q, got %q", tt.expected.DevPath, result.DevPath)
			}

			if len(result.Env) != len(tt.expected.Env) {
				t.Errorf("Env length: expected %d, got %d", len(tt.expected.Env), len(result.Env))
			}
			for k, v := range tt.expected.Env {
				if result.Env[k] != v {
					t.Errorf("Env[%q]: expected %q, got %q", k, v, result.Env[k])
				}
			}
		})
	}
}

func TestNewMonitor(t *testing.T) {
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("NewMonitor() error: %v", err)
	}
	defer func() { _ = m.Close() }()

	if m.fd <= 0 {
		t.Errorf("expected valid fd, got %d", m.fd)
	}
}

func TestMonitorClose(t *testing.T) {
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("NewMonitor() error: %v", err)
	}

	if closeErr := m.Close(); closeErr != nil {
		t.Errorf("Close() error: %v", closeErr)
	}

	// Second close should fail (bad file descriptor)
	if closeErr := m.Close(); closeErr == nil {
		t.Error("expected error on second Close()")
	}
}

func TestMonitorAddSubsystemFilter(t *testing.T) {
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("NewMonitor() error: %v", err)
	}
	defer func() { _ = m.Close() }()

	m.AddSubsystemFilter(SubsystemVideo4Linux)
	m.AddSubsystemFilter(SubsystemSound)

	if _, ok := m.filters[SubsystemVideo4Linux]; !ok {
		t.Error("expected video4linux filter to be set")
	}
	if _, ok := m.filters[SubsystemSound]; !ok {
		t.Error("expected sound filter to be set")
	}
	if _, ok := m.filters[SubsystemUSB]; ok {
		t.Error("unexpected usb filter")
	}
}

func TestMonitorRunCancellation(t *testing.T) {
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("NewMonitor() error: %v", err)
	}
	defer func() { _ = m.Close() }()

	// Use already-cancelled context - Run() should return immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	events := make(chan Event, 10)
	runErr := m.Run(ctx, events)

	if !errors.Is(runErr, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", runErr)
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are defined correctly
	if ActionAdd != "add" {
		t.Errorf("ActionAdd: expected 'add', got %q", ActionAdd)
	}
	if ActionRemove != "remove" {
		t.Errorf("ActionRemove: expected 'remove', got %q", ActionRemove)
	}
	if SubsystemVideo4Linux != "video4linux" {
		t.Errorf("SubsystemVideo4Linux: expected 'video4linux', got %q", SubsystemVideo4Linux)
	}
	if netlinkKobjectUEvent != 15 {
		t.Errorf("netlinkKobjectUEvent: expected 15, got %d", netlinkKobjectUEvent)
	}
}

// TestMonitorConcurrentFilterAdd tests for race conditions when adding filters
// concurrently. Run with: go test -race -run TestMonitorConcurrentFilterAdd.
func TestMonitorConcurrentFilterAdd(t *testing.T) {
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("NewMonitor() error: %v", err)
	}
	defer func() { _ = m.Close() }()

	// Test concurrent writes to filters map
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				m.AddSubsystemFilter(SubsystemVideo4Linux)
				m.AddSubsystemFilter(SubsystemUSB)
				m.AddSubsystemFilter(SubsystemSound)
			}
		}()
	}
	wg.Wait()

	// Verify filters were added
	m.filtersMu.RLock()
	if len(m.filters) != 3 {
		t.Errorf("expected 3 filters, got %d", len(m.filters))
	}
	m.filtersMu.RUnlock()
}

// TestParseUEventEdgeCases tests additional edge cases for uevent parsing.
func TestParseUEventEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected *Event
	}{
		{
			name:     "action only no path",
			input:    []byte("add@\x00"),
			expected: &Event{Action: "add", KObj: "", Env: map[string]string{}},
		},
		{
			name:  "very long path",
			input: []byte("add@/devices/" + strings.Repeat("a", 500) + "\x00"),
			expected: &Event{
				Action: "add",
				KObj:   "/devices/" + strings.Repeat("a", 500),
				Env:    map[string]string{},
			},
		},
		{
			name:  "key with equals in value",
			input: []byte("add@/dev/foo\x00KEY=val=ue=with=equals\x00"),
			expected: &Event{
				Action: "add",
				KObj:   "/dev/foo",
				Env:    map[string]string{"KEY": "val=ue=with=equals"},
			},
		},
		{
			name:  "consecutive null bytes in middle",
			input: []byte("add@/dev/foo\x00\x00\x00KEY=val\x00"),
			expected: &Event{
				Action: "add",
				KObj:   "/dev/foo",
				Env:    map[string]string{"KEY": "val"},
			},
		},
		{
			name:     "only null bytes",
			input:    []byte{0, 0, 0, 0},
			expected: nil,
		},
		{
			name:  "binary-looking data after valid header",
			input: []byte("add@/dev/foo\x00BINARY=\xff\xfe\xfd\x00"),
			expected: &Event{
				Action: "add",
				KObj:   "/dev/foo",
				Env:    map[string]string{"BINARY": "\xff\xfe\xfd"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseUEvent(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("expected %+v, got nil", tt.expected)
			}

			if result.Action != tt.expected.Action {
				t.Errorf("Action: expected %q, got %q", tt.expected.Action, result.Action)
			}
			if result.KObj != tt.expected.KObj {
				t.Errorf("KObj: expected %q, got %q", tt.expected.KObj, result.KObj)
			}
			for k, v := range tt.expected.Env {
				if result.Env[k] != v {
					t.Errorf("Env[%q]: expected %q, got %q", k, v, result.Env[k])
				}
			}
		})
	}
}

// TestMonitorFiltersAfterRun verifies filter behavior is consistent.
func TestMonitorFiltersAfterRun(t *testing.T) {
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("NewMonitor() error: %v", err)
	}
	defer func() { _ = m.Close() }()

	// Add filter before any activity
	m.AddSubsystemFilter(SubsystemVideo4Linux)

	// Verify filter is set
	m.filtersMu.RLock()
	_, hasV4L := m.filters[SubsystemVideo4Linux]
	m.filtersMu.RUnlock()

	if !hasV4L {
		t.Error("expected video4linux filter to be set")
	}
}
