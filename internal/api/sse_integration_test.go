package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/streams"
)

// MockStreamService for testing
type MockStreamService struct{}

func (m *MockStreamService) CreateStream(ctx context.Context, params streams.StreamCreateParams) (*streams.Stream, error) {
	return &streams.Stream{
		ID:        params.StreamID,
		DeviceID:  params.DeviceID,
		Codec:     params.Codec,
		StartTime: time.Now(),
	}, nil
}

func (m *MockStreamService) UpdateStream(ctx context.Context, streamID string, params streams.StreamUpdateParams) (*streams.Stream, error) {
	return &streams.Stream{
		ID:        streamID,
		StartTime: time.Now(),
	}, nil
}

func (m *MockStreamService) DeleteStream(ctx context.Context, streamID string) error {
	return nil
}

func (m *MockStreamService) GetStream(ctx context.Context, streamID string) (*streams.Stream, error) {
	return &streams.Stream{
		ID:        streamID,
		StartTime: time.Now(),
	}, nil
}

func (m *MockStreamService) GetStreamSpec(ctx context.Context, streamID string) (*streams.StreamSpec, error) {
	return &streams.StreamSpec{}, nil
}

func (m *MockStreamService) ListStreams(ctx context.Context) ([]streams.Stream, error) {
	return []streams.Stream{}, nil
}

func (m *MockStreamService) LoadStreamsFromConfig() error {
	return nil
}

func (m *MockStreamService) GetFFmpegCommand(ctx context.Context, streamID string, encoderOverride string) (string, bool, error) {
	return "ffmpeg command", false, nil
}

func (m *MockStreamService) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, timestamp string) {
	// No-op for mock
}

func TestSSEConnectionAndEvents(t *testing.T) {
	// Create server with mock stream service
	opts := &Options{
		AuthUsername:  "test",
		AuthPassword:  "test",
		StreamService: &MockStreamService{},
	}
	server := NewServer(opts)

	// Create test HTTP server
	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Create SSE client request with proper auth
	credentials := base64.StdEncoding.EncodeToString([]byte("test:test"))
	sseURL := fmt.Sprintf("%s/api/events?auth=%s", ts.URL, credentials)

	// Test SSE connection
	resp, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("Failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check content type
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("Expected SSE content type, got %s", resp.Header.Get("Content-Type"))
	}

	// Read initial SSE messages
	scanner := bufio.NewScanner(resp.Body)

	// Set short timeout for reading
	timeout := time.After(50 * time.Millisecond)
	messageChan := make(chan string, 10)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				messageChan <- line
			}
		}
	}()

	// Should receive initial connection message
	select {
	case msg := <-messageChan:
		if !strings.Contains(msg, "SSE connection established") {
			t.Errorf("Expected connection established message, got: %s", msg)
		}
	case <-timeout:
		t.Fatal("Timeout waiting for initial SSE message")
	}

	// Test that events are properly broadcast by triggering a stream created event
	streamData := models.StreamData{
		StreamID: "test-stream",
		DeviceID: "test-device",
		Codec:    "h264",
		Bitrate:  "2M",
	}

	// Broadcast immediately - no delay needed
	BroadcastStreamCreated(streamData, time.Now().Format(time.RFC3339))

	// Should receive the stream created event quickly
	timeout = time.After(50 * time.Millisecond)

	select {
	case msg := <-messageChan:
		if !strings.Contains(msg, "test-stream") || !strings.Contains(msg, "test-device") {
			t.Errorf("Expected stream event with test data, got: %s", msg)
		}
	case <-timeout:
		t.Fatal("Timeout waiting for stream created event")
	}

}

func TestSSEStreamUpdateEvent(t *testing.T) {
	// Create server with mock stream service
	opts := &Options{
		AuthUsername:  "test",
		AuthPassword:  "test",
		StreamService: &MockStreamService{},
	}
	server := NewServer(opts)

	// Create test HTTP server
	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Create SSE client request with proper auth
	credentials := base64.StdEncoding.EncodeToString([]byte("test:test"))
	sseURL := fmt.Sprintf("%s/api/events?auth=%s", ts.URL, credentials)

	// Test SSE connection
	resp, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("Failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read SSE messages
	scanner := bufio.NewScanner(resp.Body)
	messageChan := make(chan string, 10)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				messageChan <- line
			}
		}
	}()

	// Consume initial connection message
	timeout := time.After(50 * time.Millisecond)
	select {
	case <-messageChan:
		// Ignore initial message
	case <-timeout:
		t.Fatal("Timeout waiting for initial SSE message")
	}

	// Test stream update event by triggering a stream update
	updateStreamData := models.StreamData{
		StreamID:  "updated-stream",
		DeviceID:  "updated-device",
		Codec:     "h265",
		Bitrate:   "4M",
		StartTime: time.Now(),
	}

	// Broadcast immediately
	BroadcastStreamUpdated(updateStreamData, time.Now().Format(time.RFC3339))

	// Should receive the stream update event quickly
	timeout = time.After(50 * time.Millisecond)
	select {
	case msg := <-messageChan:
		if !strings.Contains(msg, "updated-stream") ||
			!strings.Contains(msg, "updated-device") ||
			!strings.Contains(msg, "h265") ||
			!strings.Contains(msg, "4M") {
			t.Errorf("Expected stream update event with correct data, got: %s", msg)
		}

		// Verify the event contains proper stream data structure
		if !strings.Contains(msg, `"stream":`) {
			t.Error("Stream update event should contain 'stream' field with full stream data")
		}
		if !strings.Contains(msg, `"action":"updated"`) {
			t.Error("Stream update event should contain 'action' field with 'updated' value")
		}
		if !strings.Contains(msg, `"timestamp":`) {
			t.Error("Stream update event should contain 'timestamp' field")
		}
	case <-timeout:
		t.Fatal("Timeout waiting for stream update event")
	}
}

func TestSSEStreamUpdateViaAPI(t *testing.T) {
	// Create server with mock stream service
	opts := &Options{
		AuthUsername:  "test",
		AuthPassword:  "test",
		StreamService: &MockStreamService{},
	}
	server := NewServer(opts)

	// Create test HTTP server
	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Create SSE client request with proper auth
	credentials := base64.StdEncoding.EncodeToString([]byte("test:test"))
	sseURL := fmt.Sprintf("%s/api/events?auth=%s", ts.URL, credentials)

	// Start SSE connection
	resp, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("Failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	// Read SSE messages
	scanner := bufio.NewScanner(resp.Body)
	messageChan := make(chan string, 10)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				messageChan <- line
			}
		}
	}()

	// Consume initial connection message
	timeout := time.After(50 * time.Millisecond)
	select {
	case <-messageChan:
		// Ignore initial message
	case <-timeout:
		t.Fatal("Timeout waiting for initial SSE message")
	}

	// Make API call to update stream
	updatePayload := `{"codec":"h265","bitrate":4.0}`
	req, err := http.NewRequest("PATCH", ts.URL+"/api/streams/test-stream", strings.NewReader(updatePayload))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Add basic auth header
	req.Header.Set("Authorization", "Basic "+credentials)
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	client := &http.Client{Timeout: 100 * time.Millisecond}
	apiResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to execute PATCH request: %v", err)
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 from PATCH, got %d", apiResp.StatusCode)
	}

	// Should receive the stream update event via SSE quickly
	timeout = time.After(50 * time.Millisecond)
	select {
	case msg := <-messageChan:
		if !strings.Contains(msg, "test-stream") ||
			!strings.Contains(msg, `"action":"updated"`) {
			t.Errorf("Expected stream update event from API call, got: %s", msg)
		}

		// Verify it's a proper stream event structure
		if !strings.Contains(msg, `"stream":`) {
			t.Error("API-triggered stream update event should contain 'stream' field")
		}
	case <-timeout:
		t.Fatal("Timeout waiting for API-triggered stream update event")
	}
}

func TestSSEAuthFailure(t *testing.T) {
	// Create server
	opts := &Options{
		AuthUsername:  "test",
		AuthPassword:  "test",
		StreamService: &MockStreamService{},
	}
	server := NewServer(opts)

	// Create test HTTP server
	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Test without auth
	sseURL := fmt.Sprintf("%s/api/events", ts.URL)
	resp, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected status 401, got %d", resp.StatusCode)
	}

	// Test with wrong auth
	credentials := base64.StdEncoding.EncodeToString([]byte("wrong:wrong"))
	sseURL = fmt.Sprintf("%s/api/events?auth=%s", ts.URL, credentials)
	resp, err = http.Get(sseURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 for wrong auth, got %d", resp.StatusCode)
	}
}
