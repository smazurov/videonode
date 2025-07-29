package server

import (
	"sync"
	"time"
)

// EncoderCache implements a simple cache for FFmpeg encoders
type EncoderCache struct {
	encoders         *EncoderList
	lastRefresh      time.Time
	cacheDuration    time.Duration
	mutex            sync.RWMutex
	refreshInProcess bool
}

// NewEncoderCache creates a new encoder cache with the specified cache duration
func NewEncoderCache(duration time.Duration) *EncoderCache {
	return &EncoderCache{
		cacheDuration: duration,
	}
}

// Get retrieves the encoders from the cache or refreshes them if needed
func (c *EncoderCache) Get() (*EncoderList, error) {
	c.mutex.RLock()
	// Check if we have valid cached data
	if c.encoders != nil && time.Since(c.lastRefresh) < c.cacheDuration && !c.refreshInProcess {
		defer c.mutex.RUnlock()
		return c.encoders, nil
	}
	c.mutex.RUnlock()

	// Need to refresh the cache
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check again in case another goroutine refreshed while we were waiting for the write lock
	if c.encoders != nil && time.Since(c.lastRefresh) < c.cacheDuration && !c.refreshInProcess {
		return c.encoders, nil
	}

	// Mark that a refresh is in process to prevent multiple simultaneous refreshes
	c.refreshInProcess = true
	defer func() {
		c.refreshInProcess = false
	}()

	// Fetch fresh encoder data
	encoders, err := GetFFmpegEncoders()
	if err != nil {
		// If refresh fails but we have cached data, return the stale data
		if c.encoders != nil {
			return c.encoders, nil
		}
		return nil, err
	}

	// Update cache
	c.encoders = encoders
	c.lastRefresh = time.Now()
	return encoders, nil
}

// ForceRefresh forces a refresh of the encoder cache
func (c *EncoderCache) ForceRefresh() (*EncoderList, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Mark that a refresh is in process
	c.refreshInProcess = true
	defer func() {
		c.refreshInProcess = false
	}()

	// Fetch fresh encoder data
	encoders, err := GetFFmpegEncoders()
	if err != nil {
		return nil, err
	}

	// Update cache
	c.encoders = encoders
	c.lastRefresh = time.Now()
	return encoders, nil
}

// Initialize a global encoder cache with a 5-minute cache duration
