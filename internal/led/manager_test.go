package led

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/events"
)

// Mock controller for testing
type mockController struct {
	setCalls []setCall
}

type setCall struct {
	ledType string
	enabled bool
	pattern string
}

func (m *mockController) Set(ledType string, enabled bool, pattern string) error {
	m.setCalls = append(m.setCalls, setCall{ledType, enabled, pattern})
	return nil
}

func (m *mockController) Available() []string {
	return []string{"system", "user"}
}

func (m *mockController) Patterns() []string {
	return []string{"solid", "blink"}
}

func TestManager_AllStreamsEnabled(t *testing.T) {
	ctrl := &mockController{}
	eventBus := events.New()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	mgr := NewManager(ctrl, eventBus, logger)
	mgr.Start()
	defer mgr.Stop()

	// Send events for two streams being enabled
	eventBus.Publish(events.StreamStateChangedEvent{
		StreamID:  "stream1",
		Enabled:   true,
		Timestamp: time.Now().Format(time.RFC3339),
	})
	eventBus.Publish(events.StreamStateChangedEvent{
		StreamID:  "stream2",
		Enabled:   true,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	// Give manager time to process
	time.Sleep(50 * time.Millisecond)

	// System LED should be set to solid
	if len(ctrl.setCalls) == 0 {
		t.Fatal("No LED control calls made")
	}

	lastCall := ctrl.setCalls[len(ctrl.setCalls)-1]
	if lastCall.pattern != "solid" {
		t.Errorf("Expected solid pattern when all enabled, got %q", lastCall.pattern)
	}
}

func TestManager_SomeStreamsDisabled(t *testing.T) {
	ctrl := &mockController{}
	eventBus := events.New()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	mgr := NewManager(ctrl, eventBus, logger)
	mgr.Start()
	defer mgr.Stop()

	// Enable two streams, then disable one
	eventBus.Publish(events.StreamStateChangedEvent{
		StreamID:  "stream1",
		Enabled:   true,
		Timestamp: time.Now().Format(time.RFC3339),
	})
	eventBus.Publish(events.StreamStateChangedEvent{
		StreamID:  "stream2",
		Enabled:   true,
		Timestamp: time.Now().Format(time.RFC3339),
	})
	eventBus.Publish(events.StreamStateChangedEvent{
		StreamID:  "stream2",
		Enabled:   false,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	// Give manager time to process
	time.Sleep(50 * time.Millisecond)

	// System LED should be set to blink
	if len(ctrl.setCalls) == 0 {
		t.Fatal("No LED control calls made")
	}

	lastCall := ctrl.setCalls[len(ctrl.setCalls)-1]
	if lastCall.pattern != "blink" {
		t.Errorf("Expected blink pattern when some disabled, got %q", lastCall.pattern)
	}
}

func TestManager_GetController(t *testing.T) {
	ctrl := &mockController{}
	eventBus := events.New()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	mgr := NewManager(ctrl, eventBus, logger)

	if got := mgr.GetController(); got != ctrl {
		t.Error("GetController() did not return the original controller")
	}
}
