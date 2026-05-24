package streaming

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/smazurov/videonode/internal/logging"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(logging.GetLogger("streaming-test"))
}

// injectStream registers a stream directly (bypassing the gortsplib OnAnnounce
// path which needs a real session/conn). Lets unit tests exercise the
// callback wiring without a real RTSP producer.
func injectStream(s *Server, id string) *Stream {
	stream := NewStream(id, &description.Session{}, s.logger)
	stream.SetOnNoReaders(s.handleLastReaderGone)

	s.mu.Lock()
	s.streams[id] = stream
	s.mu.Unlock()
	return stream
}

func TestEnsureStreamReady_ExistingStreamReturnsImmediately(t *testing.T) {
	s := newTestServer(t)
	want := injectStream(s, "live")

	var ensured int32
	s.SetOnEnsureStream(func(string) error { atomic.AddInt32(&ensured, 1); return nil })

	got := s.EnsureStreamReady("live", time.Second)
	if got != want {
		t.Fatalf("EnsureStreamReady returned %v, want existing stream", got)
	}
	if n := atomic.LoadInt32(&ensured); n != 0 {
		t.Fatalf("ensure hook called %d times, want 0 (stream already registered)", n)
	}
}

func TestEnsureStreamReady_InvokesEnsureHookAndPolls(t *testing.T) {
	s := newTestServer(t)

	var calls int32
	s.SetOnEnsureStream(func(id string) error {
		atomic.AddInt32(&calls, 1)
		// Simulate the encoder publishing the stream slightly later.
		go func() {
			time.Sleep(80 * time.Millisecond)
			injectStream(s, id)
		}()
		return nil
	})

	start := time.Now()
	got := s.EnsureStreamReady("delayed", time.Second)
	elapsed := time.Since(start)

	if got == nil {
		t.Fatalf("EnsureStreamReady returned nil, expected stream after lazy-start")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("ensure hook called %d times, want 1", n)
	}
	if elapsed < 80*time.Millisecond {
		t.Fatalf("returned in %v, expected to wait at least 80ms", elapsed)
	}
}

func TestEnsureStreamReady_HookErrorReturnsNil(t *testing.T) {
	s := newTestServer(t)

	s.SetOnEnsureStream(func(string) error { return errors.New("boom") })

	got := s.EnsureStreamReady("missing", 200*time.Millisecond)
	if got != nil {
		t.Fatalf("EnsureStreamReady returned %v, want nil on hook error", got)
	}
}

func TestEnsureStreamReady_TimesOutWhenNotPublished(t *testing.T) {
	s := newTestServer(t)
	s.SetOnEnsureStream(func(string) error { return nil })

	start := time.Now()
	got := s.EnsureStreamReady("never", 150*time.Millisecond)
	elapsed := time.Since(start)

	if got != nil {
		t.Fatalf("EnsureStreamReady returned %v, want nil after timeout", got)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("returned in %v, expected to wait full timeout 150ms", elapsed)
	}
}

func TestLastReaderGone_FiresAfterDebounce(t *testing.T) {
	s := newTestServer(t)
	stream := injectStream(s, "live")

	var fired int32
	done := make(chan struct{})
	s.SetOnLastReaderGone(func(id string) {
		if id == "live" {
			atomic.StoreInt32(&fired, 1)
			close(done)
		}
	})

	// Drive a reader through its lifecycle so onNoReaders fires.
	r := NewReader(stream, "reader-1")
	r.Close()

	// Should NOT have fired yet — debounce is 2s.
	time.Sleep(200 * time.Millisecond)
	if atomic.LoadInt32(&fired) != 0 {
		t.Fatalf("onLastReaderGone fired before debounce window")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("onLastReaderGone never fired after debounce")
	}
}

func TestLastReaderGone_CancelledByReconnect(t *testing.T) {
	s := newTestServer(t)
	stream := injectStream(s, "live")

	var fired int32
	s.SetOnLastReaderGone(func(string) { atomic.AddInt32(&fired, 1) })

	r1 := NewReader(stream, "reader-1")
	r1.Close()

	// Reconnect well before the 2s debounce expires; EnsureStreamReady
	// finds the existing stream and cancels the pending timer.
	time.Sleep(100 * time.Millisecond)
	if got := s.EnsureStreamReady("live", time.Second); got != stream {
		t.Fatalf("EnsureStreamReady returned %v, want existing stream", got)
	}
	r2 := NewReader(stream, "reader-2")

	// Wait past the original debounce — should still not have fired.
	time.Sleep(2500 * time.Millisecond)
	if n := atomic.LoadInt32(&fired); n != 0 {
		t.Fatalf("onLastReaderGone fired %d times despite reconnect", n)
	}

	// Cleanup: closing r2 should re-arm and eventually fire.
	r2.Close()
}

func TestLastReaderGone_NoFireIfReaderReattachedBeforeTimer(t *testing.T) {
	s := newTestServer(t)
	stream := injectStream(s, "live")

	var fired int32
	s.SetOnLastReaderGone(func(string) { atomic.AddInt32(&fired, 1) })

	r1 := NewReader(stream, "r1")
	r1.Close()
	// Re-attach a reader without calling EnsureStreamReady (so the
	// debouncer wasn't cancelled). The timer must skip firing because
	// stream.ReaderCount() > 0 by the time it fires.
	NewReader(stream, "r2")

	time.Sleep(2500 * time.Millisecond)
	if n := atomic.LoadInt32(&fired); n != 0 {
		t.Fatalf("onLastReaderGone fired %d times despite re-attached reader", n)
	}
}

func TestServerSetters_AreSafeForConcurrentUse(t *testing.T) {
	s := newTestServer(t)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 50 {
				s.SetOnLastReaderGone(func(string) {})
				s.SetOnEnsureStream(func(string) error { return nil })
			}
		})
	}
	wg.Wait()
}
