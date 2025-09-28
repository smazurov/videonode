package mediamtx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// Client is an HTTP client for the MediaMTX API
type Client struct {
	baseURL      string
	httpClient   *http.Client
	syncCallback func() []*ProcessedStream
	logger       *slog.Logger

	// Health monitoring
	healthTicker *time.Ticker
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// ProcessedStream represents a stream ready to be added to MediaMTX
type ProcessedStream struct {
	StreamID      string
	FFmpegCommand string
}

// PathConfig represents the configuration for a MediaMTX path
type pathConfigRequest struct {
	RunOnInit        string `json:"runOnInit"`
	RunOnInitRestart bool   `json:"runOnInitRestart"`
}

// PathInfo represents information about a MediaMTX path
type PathInfo struct {
	Name   string `json:"name"`
	Source struct {
		Type string `json:"type"`
	} `json:"source"`
	Ready bool `json:"ready"`
}

// PathListResponse represents the response from the paths list endpoint
type PathListResponse struct {
	ItemCount int         `json:"itemCount"`
	Items     []*PathInfo `json:"items"`
}

// wrapWithLogger wraps an ffmpeg command with systemd logging if enabled
func wrapWithLogger(command string) string {
	if !globalConfig.EnableLogging {
		return command
	}

	// Escape single quotes in the command by replacing ' with '\''
	escapedCmd := strings.ReplaceAll(command, "'", "'\"'\"'")
	return fmt.Sprintf("/bin/bash -c '%s 2>&1 | systemd-cat -t ffmpeg-$MTX_PATH'", escapedCmd)
}

// NewClient creates a new MediaMTX API client
func NewClient(baseURL string, syncCallback func() []*ProcessedStream) *Client {
	return &Client{
		baseURL:      baseURL,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		syncCallback: syncCallback,
		logger:       logging.GetLogger("mediamtx"),
		stopChan:     make(chan struct{}),
	}
}

// StartHealthMonitor starts the health monitoring goroutine
func (c *Client) StartHealthMonitor() {
	c.healthTicker = time.NewTicker(1 * time.Second)
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()
		wasDown := true

		for {
			select {
			case <-c.healthTicker.C:
				if c.isAvailable() {
					if wasDown {
						c.logger.Info("MediaMTX API is back online, syncing all streams")
						if err := c.SyncAll(); err != nil {
							c.logger.Error("Failed to sync streams", "error", err)
						}
					}
					wasDown = false
				} else {
					if !wasDown {
						c.logger.Warn("MediaMTX API is unavailable")
					}
					wasDown = true
				}
			case <-c.stopChan:
				return
			}
		}
	}()
}

// Stop stops the health monitoring
func (c *Client) Stop() {
	if c.healthTicker != nil {
		c.healthTicker.Stop()
	}
	close(c.stopChan)
	c.wg.Wait()
}

// isAvailable checks if the MediaMTX API is reachable
func (c *Client) isAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v3/paths/list", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// AddPath adds a new path to MediaMTX
func (c *Client) AddPath(streamID, ffmpegCommand string) error {
	config := pathConfigRequest{
		RunOnInit:        wrapWithLogger(ffmpegCommand),
		RunOnInitRestart: true,
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal path config: %w", err)
	}

	url := fmt.Sprintf("%s/v3/config/paths/add/%s", c.baseURL, streamID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add path: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to add path, status: %d", resp.StatusCode)
	}

	c.logger.Info("Added path to MediaMTX", "path", streamID)
	return nil
}

// RestartPath forces a restart of the FFmpeg process by deleting and recreating the path
// This should only be used during startup to ensure fresh connections to progress sockets
func (c *Client) RestartPath(streamID, ffmpegCommand string) error {
	// First, delete the existing path (ignore errors if it doesn't exist)
	_ = c.DeletePath(streamID)

	// Small delay to ensure cleanup
	time.Sleep(100 * time.Millisecond)

	// Recreate the path with fresh FFmpeg process
	return c.AddPath(streamID, ffmpegCommand)
}

// UpdatePath updates an existing path in MediaMTX
func (c *Client) UpdatePath(streamID, ffmpegCommand string) error {
	config := pathConfigRequest{
		RunOnInit:        wrapWithLogger(ffmpegCommand),
		RunOnInitRestart: true,
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal path config: %w", err)
	}

	url := fmt.Sprintf("%s/v3/config/paths/patch/%s", c.baseURL, streamID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update path: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update path, status: %d", resp.StatusCode)
	}

	c.logger.Info("Updated path in MediaMTX", "path", streamID)
	return nil
}

// DeletePath removes a path from MediaMTX
func (c *Client) DeletePath(streamID string) error {
	url := fmt.Sprintf("%s/v3/config/paths/delete/%s", c.baseURL, streamID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete path: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete path, status: %d", resp.StatusCode)
	}

	c.logger.Info("Deleted path from MediaMTX", "path", streamID)
	return nil
}

// ListPaths returns all paths currently in MediaMTX
func (c *Client) ListPaths() ([]*PathInfo, error) {
	url := fmt.Sprintf("%s/v3/paths/list", c.baseURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to list paths: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list paths, status: %d", resp.StatusCode)
	}

	var pathList PathListResponse
	if err := json.NewDecoder(resp.Body).Decode(&pathList); err != nil {
		return nil, fmt.Errorf("failed to decode path list: %w", err)
	}

	return pathList.Items, nil
}

// SyncAll synchronizes all streams from the repository with MediaMTX
func (c *Client) SyncAll() error {
	if c.syncCallback == nil {
		return fmt.Errorf("sync callback not configured")
	}

	// Get desired streams from callback
	streams := c.syncCallback()

	// Get existing paths from MediaMTX
	existing, err := c.ListPaths()
	if err != nil {
		return fmt.Errorf("failed to list existing paths: %w", err)
	}

	// Build map of existing paths for easy lookup
	existingMap := make(map[string]bool)
	for _, path := range existing {
		existingMap[path.Name] = true
	}

	// Build map of desired streams
	desiredMap := make(map[string]*ProcessedStream)
	for _, stream := range streams {
		desiredMap[stream.StreamID] = stream
	}

	// Add or update streams
	for _, stream := range streams {
		if existingMap[stream.StreamID] {
			// Path exists, update it
			if err := c.UpdatePath(stream.StreamID, stream.FFmpegCommand); err != nil {
				c.logger.Error("Failed to update path", "path", stream.StreamID, "error", err)
			}
		} else {
			// Path doesn't exist, add it
			if err := c.AddPath(stream.StreamID, stream.FFmpegCommand); err != nil {
				c.logger.Error("Failed to add path", "path", stream.StreamID, "error", err)
			}
		}
	}

	// Delete paths that shouldn't exist
	for _, path := range existing {
		if _, shouldExist := desiredMap[path.Name]; !shouldExist {
			if err := c.DeletePath(path.Name); err != nil {
				c.logger.Error("Failed to delete path", "path", path.Name, "error", err)
			}
		}
	}

	c.logger.Info("Synced streams with MediaMTX", "count", len(streams))
	return nil
}
