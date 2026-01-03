package nats

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestServerStartStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := NewServer(ServerOptions{
		Port:   14222, // Use non-default port for testing
		Name:   "test-server",
		Logger: logger,
	})

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	if !server.IsRunning() {
		t.Error("Server should be running after Start()")
	}

	url := server.ClientURL()
	if url == "" {
		t.Error("ClientURL should not be empty")
	}

	server.Stop()

	if server.IsRunning() {
		t.Error("Server should not be running after Stop()")
	}
}

func TestStreamClientGracefulDegradation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create client with non-existent server
	client := NewStreamClient("nats://localhost:59999", "test-stream", logger)

	// Connect should fail but not panic
	err := client.Connect()
	if err == nil {
		t.Error("Connect should fail with non-existent server")
	}

	// These should be no-ops without panicking
	client.PublishMetrics(MetricsMessage{StreamID: "test"})
	client.PublishLog(LogMessage{StreamID: "test", Message: "test"})
	client.PublishState(StateMessage{StreamID: "test", Enabled: true})

	if client.IsConnected() {
		t.Error("Client should not be connected")
	}

	client.Close()
}

func TestStreamClientConnectAndPublish(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Start a test server
	server := NewServer(ServerOptions{
		Port:   14223,
		Name:   "test-server",
		Logger: logger,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Create and connect client
	client := NewStreamClient(server.ClientURL(), "test-stream", logger)
	if err := client.Connect(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Error("Client should be connected")
	}

	// Publish metrics (no error expected)
	client.PublishMetrics(MetricsMessage{
		StreamID:        "test-stream",
		Timestamp:       time.Now().Format(time.RFC3339),
		FPS:             "30",
		DroppedFrames:   "0",
		DuplicateFrames: "0",
		ProcessingSpeed: "1.0x",
	})

	// Publish log
	client.PublishLog(LogMessage{
		StreamID:  "test-stream",
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     "info",
		Message:   "Test log message",
		Source:    "test",
	})

	// Publish state
	client.PublishState(StateMessage{
		StreamID:  "test-stream",
		Timestamp: time.Now().Format(time.RFC3339),
		Enabled:   true,
		Reason:    "test",
	})
}

func TestControlPublisher(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Start a test server
	server := NewServer(ServerOptions{
		Port:   14224,
		Name:   "test-server",
		Logger: logger,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Create control publisher
	publisher, err := NewControlPublisher(server.ClientURL(), logger)
	if err != nil {
		t.Fatalf("Failed to create control publisher: %v", err)
	}
	defer publisher.Close()

	// Send restart command (no error expected)
	if err := publisher.Restart("test-stream", "test"); err != nil {
		t.Errorf("Restart failed: %v", err)
	}
}

func TestStreamClientRestartHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Start a test server
	server := NewServer(ServerOptions{
		Port:   14225,
		Name:   "test-server",
		Logger: logger,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Create and connect stream client
	client := NewStreamClient(server.ClientURL(), "test-stream", logger)
	if err := client.Connect(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Set up restart handler
	restartCalled := make(chan struct{}, 1)
	client.OnRestart(func() {
		restartCalled <- struct{}{}
	})

	// Create control publisher
	publisher, err := NewControlPublisher(server.ClientURL(), logger)
	if err != nil {
		t.Fatalf("Failed to create control publisher: %v", err)
	}
	defer publisher.Close()

	// Send restart command
	if err := publisher.Restart("test-stream", "test"); err != nil {
		t.Fatalf("Restart failed: %v", err)
	}

	// Wait for restart handler to be called
	select {
	case <-restartCalled:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Restart handler was not called within timeout")
	}
}

func TestMessageMarshalUnmarshal(t *testing.T) {
	t.Run("MetricsMessage", func(t *testing.T) {
		original := MetricsMessage{
			StreamID:        "test-stream",
			Timestamp:       "2024-01-01T00:00:00Z",
			FPS:             "30",
			DroppedFrames:   "5",
			DuplicateFrames: "2",
			ProcessingSpeed: "1.0x",
		}

		data, err := original.Marshal()
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		parsed, err := UnmarshalMetrics(data)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if parsed.StreamID != original.StreamID {
			t.Errorf("StreamID mismatch: got %s, want %s", parsed.StreamID, original.StreamID)
		}
		if parsed.FPS != original.FPS {
			t.Errorf("FPS mismatch: got %s, want %s", parsed.FPS, original.FPS)
		}
	})

	t.Run("ControlMessage", func(t *testing.T) {
		original := ControlMessage{
			Action:    "restart",
			StreamID:  "test-stream",
			Timestamp: "2024-01-01T00:00:00Z",
			Reason:    "test",
		}

		data, err := original.Marshal()
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		parsed, err := UnmarshalControl(data)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if parsed.Action != original.Action {
			t.Errorf("Action mismatch: got %s, want %s", parsed.Action, original.Action)
		}
		if parsed.Reason != original.Reason {
			t.Errorf("Reason mismatch: got %s, want %s", parsed.Reason, original.Reason)
		}
	})
}

func TestSubjectFunctions(t *testing.T) {
	tests := []struct {
		fn       func(string) string
		streamID string
		expected string
	}{
		{SubjectStreamMetrics, "stream-001", "videonode.streams.stream-001.metrics"},
		{SubjectStreamLogs, "stream-001", "videonode.streams.stream-001.logs"},
		{SubjectStreamState, "stream-001", "videonode.streams.stream-001.state"},
		{SubjectControlRestart, "stream-001", "videonode.control.stream-001.restart"},
	}

	for _, tt := range tests {
		result := tt.fn(tt.streamID)
		if result != tt.expected {
			t.Errorf("Got %s, want %s", result, tt.expected)
		}
	}
}
