package logging

import (
	"sync"
	"time"
)

// LogEntry represents a single log line stored in the ring buffer.
type LogEntry struct {
	Timestamp  time.Time      `json:"timestamp"`
	Level      string         `json:"level"`
	Module     string         `json:"module"`
	Message    string         `json:"message"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// RingBuffer is a thread-safe circular buffer for log entries.
type RingBuffer struct {
	entries []LogEntry
	size    int
	head    int
	count   int
	mu      sync.RWMutex
}

// NewRingBuffer creates a new ring buffer with the specified capacity.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		entries: make([]LogEntry, size),
		size:    size,
	}
}

// Write adds a log entry to the buffer, overwriting the oldest entry if full.
func (rb *RingBuffer) Write(entry LogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.entries[rb.head] = entry
	rb.head = (rb.head + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	}
}

// ReadAll returns all entries in chronological order.
func (rb *RingBuffer) ReadAll() []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return nil
	}

	result := make([]LogEntry, rb.count)

	if rb.count < rb.size {
		// Buffer not full yet, entries start at 0
		copy(result, rb.entries[:rb.count])
	} else {
		// Buffer is full, oldest entry is at head
		firstPart := rb.entries[rb.head:]
		secondPart := rb.entries[:rb.head]
		copy(result, firstPart)
		copy(result[len(firstPart):], secondPart)
	}

	return result
}

// UpdateLatest walks backward from the most recent entry, finds the first
// entry matching the predicate, and applies the update function in-place.
// Returns true if a matching entry was found and updated.
func (rb *RingBuffer) UpdateLatest(match func(*LogEntry) bool, update func(*LogEntry)) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for i := range rb.count {
		idx := (rb.head - 1 - i + rb.size) % rb.size
		if match(&rb.entries[idx]) {
			update(&rb.entries[idx])
			return true
		}
	}
	return false
}

// Count returns the number of entries in the buffer.
func (rb *RingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}
