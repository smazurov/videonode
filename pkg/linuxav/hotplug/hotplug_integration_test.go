//go:build linux && integration

package hotplug

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestMonitorIntegration is a manual test that requires actual device events.
// Run with: go test -tags=integration -v -run TestMonitorIntegration -timeout 60s
// Then plug/unplug a USB device within the timeout.
func TestMonitorIntegration(t *testing.T) {
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("NewMonitor() error: %v", err)
	}
	defer func() { _ = m.Close() }()

	// Filter for video4linux and usb to catch common test scenarios
	m.AddSubsystemFilter(SubsystemVideo4Linux)
	m.AddSubsystemFilter(SubsystemUSB)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	events := make(chan Event, 10)
	go func() {
		if runErr := m.Run(ctx, events); runErr != nil && !errors.Is(runErr, context.DeadlineExceeded) && !errors.Is(runErr, context.Canceled) {
			t.Logf("Run() error: %v", runErr)
		}
	}()

	t.Log("Waiting for device events... plug/unplug a USB device")

	select {
	case event := <-events:
		t.Logf("Received event: Action=%s Subsystem=%s DevName=%s KObj=%s",
			event.Action, event.Subsystem, event.DevName, event.KObj)
	case <-ctx.Done():
		t.Log("No events received (this is expected if no devices were plugged/unplugged)")
	}
}
