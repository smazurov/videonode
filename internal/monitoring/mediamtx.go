package monitoring

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MediaMTXPath represents a stream path in MediaMTX
type MediaMTXPath struct {
	Name   string `json:"name"`
	Source struct {
		Type string `json:"type"`
	} `json:"source"`
	Ready  bool `json:"ready"`
	Tracks int  `json:"tracks"`
}

// MediaMTXPathsResponse represents the response from MediaMTX paths API
type MediaMTXPathsResponse struct {
	Items map[string]*MediaMTXPath `json:"items"`
}

// MediaMTXHealthCheck represents the health status of MediaMTX
type MediaMTXHealthCheck struct {
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	StreamCount int       `json:"stream_count"`
	Timestamp   time.Time `json:"timestamp"`
}

// MediaMTXMonitor monitors MediaMTX health and status
type MediaMTXMonitor struct {
	apiURL string
}

// NewMediaMTXMonitor creates a new MediaMTX monitor
func NewMediaMTXMonitor(apiPort int) *MediaMTXMonitor {
	return &MediaMTXMonitor{
		apiURL: fmt.Sprintf("http://127.0.0.1:%d", apiPort),
	}
}

// CheckHealth performs a health check on MediaMTX
func (m *MediaMTXMonitor) CheckHealth() (*MediaMTXHealthCheck, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Call the MediaMTX paths API
	resp, err := client.Get(fmt.Sprintf("%s/v1/paths/list", m.apiURL))
	if err != nil {
		return &MediaMTXHealthCheck{
			Status:      "error",
			Message:     fmt.Sprintf("Failed to connect to MediaMTX: %v", err),
			StreamCount: 0,
			Timestamp:   time.Now(),
		}, err
	}
	defer resp.Body.Close()

	// Check if response is OK
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &MediaMTXHealthCheck{
			Status:      "error",
			Message:     fmt.Sprintf("MediaMTX returned status %d: %s", resp.StatusCode, string(body)),
			StreamCount: 0,
			Timestamp:   time.Now(),
		}, fmt.Errorf("MediaMTX returned status %d", resp.StatusCode)
	}

	// Parse the response
	var pathsResp MediaMTXPathsResponse
	if err := json.NewDecoder(resp.Body).Decode(&pathsResp); err != nil {
		return &MediaMTXHealthCheck{
			Status:      "error",
			Message:     fmt.Sprintf("Failed to parse MediaMTX response: %v", err),
			StreamCount: 0,
			Timestamp:   time.Now(),
		}, err
	}

	// Count the streams
	streamCount := len(pathsResp.Items)

	return &MediaMTXHealthCheck{
		Status:      "ok",
		Message:     "MediaMTX is healthy",
		StreamCount: streamCount,
		Timestamp:   time.Now(),
	}, nil
}

// GetStreamPaths returns the list of configured stream paths
func (m *MediaMTXMonitor) GetStreamPaths() ([]string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("%s/v1/paths/list", m.apiURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MediaMTX returned status %d", resp.StatusCode)
	}

	var pathsResp MediaMTXPathsResponse
	if err := json.NewDecoder(resp.Body).Decode(&pathsResp); err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(pathsResp.Items))
	for name := range pathsResp.Items {
		paths = append(paths, name)
	}

	return paths, nil
}