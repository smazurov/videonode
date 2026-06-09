package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// recordingHandler captures handled records for test assertions.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *recordingHandler) getRecords() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(h.records))
	copy(out, h.records)
	return out
}

func makeRecord(t time.Time, msg string) slog.Record {
	return slog.NewRecord(t, slog.LevelInfo, msg, 0)
}

func TestDedupHandler_FirstMessageForwarded(t *testing.T) {
	rec := &recordingHandler{}
	h := NewDedupHandler(rec, time.Minute)

	r := makeRecord(time.Now(), "hello")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	records := rec.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Message != "hello" {
		t.Errorf("expected message 'hello', got %q", records[0].Message)
	}
}

func TestDedupHandler_SuppressesDuplicates(t *testing.T) {
	rec := &recordingHandler{}
	h := NewDedupHandler(rec, time.Minute)

	now := time.Now()
	for i := range 10 {
		r := makeRecord(now.Add(time.Duration(i)*time.Millisecond), "spam")
		_ = h.Handle(context.Background(), r)
	}

	// Only the first should be forwarded to inner handler
	records := rec.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record forwarded to inner handler, got %d", len(records))
	}
}

func TestDedupHandler_ForwardsAfterCooldown(t *testing.T) {
	rec := &recordingHandler{}
	cooldown := 100 * time.Millisecond
	h := NewDedupHandler(rec, cooldown)

	now := time.Now()

	// First message
	_ = h.Handle(context.Background(), makeRecord(now, "msg"))

	// 5 suppressed
	for i := 1; i <= 5; i++ {
		_ = h.Handle(context.Background(), makeRecord(now.Add(time.Duration(i)*time.Millisecond), "msg"))
	}

	// After cooldown — should forward as new first occurrence
	_ = h.Handle(context.Background(), makeRecord(now.Add(cooldown+time.Millisecond), "msg"))

	records := rec.getRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

func TestDedupHandler_DifferentMessagesNotSuppressed(t *testing.T) {
	rec := &recordingHandler{}
	h := NewDedupHandler(rec, time.Minute)

	now := time.Now()
	_ = h.Handle(context.Background(), makeRecord(now, "msg-a"))
	_ = h.Handle(context.Background(), makeRecord(now, "msg-b"))
	_ = h.Handle(context.Background(), makeRecord(now, "msg-c"))

	records := rec.getRecords()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
}

func TestDedupHandler_SameMessageDifferentAttrsNotSuppressed(t *testing.T) {
	rec := &recordingHandler{}
	h := NewDedupHandler(rec, time.Minute)

	now := time.Now()
	for i := range 3 {
		r := makeRecord(now.Add(time.Duration(i)*time.Millisecond), "frame")
		r.AddAttrs(slog.Int("n", i))
		_ = h.Handle(context.Background(), r)
	}

	// Identical message but distinct attributes => distinct keys => none folded.
	if records := rec.getRecords(); len(records) != 3 {
		t.Fatalf("expected 3 records (distinct attrs), got %d", len(records))
	}
}

func TestDedupHandler_LiveCallbackUpdates(t *testing.T) {
	// Set up global buffer and callback to verify live updates
	buf := NewRingBuffer(100)
	var callbackEntries []LogEntry
	var cbMu sync.Mutex

	mutex.Lock()
	oldBuffer := logBuffer
	oldCallback := logCallback
	logBuffer = buf
	logCallback = func(entry LogEntry) {
		cbMu.Lock()
		callbackEntries = append(callbackEntries, entry)
		cbMu.Unlock()
	}
	mutex.Unlock()

	defer func() {
		mutex.Lock()
		logBuffer = oldBuffer
		logCallback = oldCallback
		mutex.Unlock()
	}()

	rec := &recordingHandler{}
	h := NewDedupHandler(rec, time.Minute)

	now := time.Now()

	// First message — goes through inner handler (BufferHandler would write it)
	_ = h.Handle(context.Background(), makeRecord(now, "spam"))

	// Simulate what BufferHandler would do: write to ring buffer
	buf.Write(LogEntry{
		Timestamp:  now,
		Level:      "info",
		Module:     "app",
		Message:    "spam",
		Attributes: map[string]any{},
	})

	// 3 duplicates — should trigger live callback updates
	for i := 1; i <= 3; i++ {
		_ = h.Handle(context.Background(), makeRecord(now.Add(time.Duration(i)*time.Millisecond), "spam"))
	}

	// Inner handler should only have 1 record (first occurrence)
	records := rec.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record in inner handler, got %d", len(records))
	}

	// Callback should have been called 3 times for updates
	cbMu.Lock()
	cbCount := len(callbackEntries)
	lastEntry := callbackEntries[cbCount-1]
	cbMu.Unlock()

	if cbCount != 3 {
		t.Fatalf("expected 3 callback updates, got %d", cbCount)
	}

	// Last callback should have suppressed=3
	if lastEntry.Attributes["suppressed"] != 3 {
		t.Errorf("expected suppressed=3, got %v", lastEntry.Attributes["suppressed"])
	}

	// Ring buffer entry should be updated in-place
	entries := buf.ReadAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 ring buffer entry, got %d", len(entries))
	}
	if entries[0].Attributes["suppressed"] != 3 {
		t.Errorf("expected ring buffer suppressed=3, got %v", entries[0].Attributes["suppressed"])
	}
}

func TestDedupHandler_PublishedAttributesNotMutated(t *testing.T) {
	// A published Attributes map must never be mutated afterwards: an SSE
	// goroutine json-marshals it while the handler updates suppression. The
	// real BufferHandler as inner reproduces the shared-map case.
	buf := NewRingBuffer(100)
	var cbMu sync.Mutex
	var delivered []LogEntry

	mutex.Lock()
	oldBuffer, oldCallback := logBuffer, logCallback
	logBuffer = buf
	logCallback = func(e LogEntry) {
		cbMu.Lock()
		delivered = append(delivered, e)
		cbMu.Unlock()
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		logBuffer = oldBuffer
		logCallback = oldCallback
		mutex.Unlock()
	}()

	h := NewDedupHandler(NewBufferHandler(slog.LevelInfo), time.Minute)

	now := time.Now()
	first := makeRecord(now, "spam")
	first.AddAttrs(slog.String("k", "v"))
	_ = h.Handle(context.Background(), first)

	cbMu.Lock()
	published := delivered[0].Attributes
	cbMu.Unlock()

	// Subsequent duplicates update suppression state on the ring buffer entry.
	// They must carry the same attribute as the first record, or full-key dedup
	// treats them as distinct records and never folds.
	for i := 1; i <= 3; i++ {
		dup := makeRecord(now.Add(time.Duration(i)*time.Millisecond), "spam")
		dup.AddAttrs(slog.String("k", "v"))
		_ = h.Handle(context.Background(), dup)
	}

	if v, mutated := published["suppressed"]; mutated {
		t.Fatalf("published Attributes map was mutated in place (gained suppressed=%v); "+
			"an SSE encoder marshaling this map races with the suppression update", v)
	}
}

func TestDedupHandler_ConcurrentStreamWhileSuppressing(t *testing.T) {
	// Regression for the SSE log panic ("index out of range" in mapEncoder):
	// marshaling ReadAll snapshots (which share Attributes maps by reference)
	// raced with suppression updates. Run under `go test -race`.
	buf := NewRingBuffer(100)

	mutex.Lock()
	oldBuffer, oldCallback := logBuffer, logCallback
	logBuffer = buf
	logCallback = func(LogEntry) {}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		logBuffer = oldBuffer
		logCallback = oldCallback
		mutex.Unlock()
	}()

	h := NewDedupHandler(NewBufferHandler(slog.LevelInfo), time.Hour)

	base := time.Now()
	seed := makeRecord(base, "spam")
	seed.AddAttrs(slog.String("a", "1"), slog.String("b", "2"))
	_ = h.Handle(context.Background(), seed)

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: duplicates within cooldown -> UpdateLatest swaps the slot's map.
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
				dup := makeRecord(base.Add(time.Duration(i)*time.Microsecond), "spam")
				dup.AddAttrs(slog.String("a", "1"), slog.String("b", "2"))
				_ = h.Handle(context.Background(), dup)
			}
		}
	}()

	// Reader: marshal ReadAll snapshots (the SSE streaming path).
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				for _, e := range buf.ReadAll() {
					if _, err := json.Marshal(e); err != nil {
						t.Errorf("marshal: %v", err)
						return
					}
				}
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()
}

func TestDedupHandler_StaleEntryCleanup(t *testing.T) {
	rec := &recordingHandler{}
	cooldown := 50 * time.Millisecond
	h := NewDedupHandler(rec, cooldown)

	now := time.Now()

	// Create entries for two messages
	_ = h.Handle(context.Background(), makeRecord(now, "old-msg"))
	_ = h.Handle(context.Background(), makeRecord(now, "new-msg"))

	// Wait for 2x cooldown so "old-msg" becomes stale
	later := now.Add(3 * cooldown)
	_ = h.Handle(context.Background(), makeRecord(later, "trigger-cleanup"))

	// old-msg and new-msg should be cleaned up, only trigger-cleanup remains
	h.mu.Lock()
	seenCount := len(h.seen)
	h.mu.Unlock()

	if seenCount != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", seenCount)
	}
}
