package snapshots

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeFetcher struct {
	mu          sync.Mutex
	srcCalls    int
	compCalls   int
	srcFrame    Frame
	srcErr      error
	compFrame   Frame
	compErr     error
	delay       time.Duration
	frameIdxSeq atomic.Uint64
}

func (f *fakeFetcher) SnapshotSource(ctx context.Context, _ string) (Frame, error) {
	f.mu.Lock()
	f.srcCalls++
	d := f.delay
	out := f.srcFrame
	err := f.srcErr
	f.mu.Unlock()
	if d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return Frame{}, ctx.Err()
		}
	}
	if err != nil {
		return Frame{}, err
	}
	out.FrameIdx = f.frameIdxSeq.Add(1)
	return out, nil
}

func (f *fakeFetcher) SnapshotComposer(_ context.Context, _ string) (Frame, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.compCalls++
	if f.compErr != nil {
		return Frame{}, f.compErr
	}
	out := f.compFrame
	out.FrameIdx = f.frameIdxSeq.Add(1)
	return out, nil
}

type fakeEncoder struct {
	calls atomic.Uint64
}

func (e *fakeEncoder) EncodeJPEG(f Frame) ([]byte, error) {
	e.calls.Add(1)
	return []byte("JPEG:" + string(f.Bytes)), nil
}

func makeFrame(payload string) Frame {
	return Frame{Bytes: []byte(payload), Format: FormatNV12, Width: 4, Height: 4}
}

func TestCache_Get_ReturnsFreshOnFirstCall(t *testing.T) {
	fet := &fakeFetcher{srcFrame: makeFrame("hi")}
	enc := &fakeEncoder{}
	c := NewCache(Config{StaleAfter: 100 * time.Millisecond}, fet, enc)

	e, err := c.Get(context.Background(), KindSource, "id1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(e.JPEG) != "JPEG:hi" {
		t.Fatalf("jpeg=%q", e.JPEG)
	}
	if e.FrameIdx != 1 {
		t.Fatalf("frame idx=%d", e.FrameIdx)
	}
	if fet.srcCalls != 1 {
		t.Fatalf("srcCalls=%d", fet.srcCalls)
	}
}

func TestCache_Get_ServesCachedWithinStaleWindow(t *testing.T) {
	fet := &fakeFetcher{srcFrame: makeFrame("hi")}
	enc := &fakeEncoder{}
	c := NewCache(Config{StaleAfter: 5 * time.Second}, fet, enc)

	for range 5 {
		if _, err := c.Get(context.Background(), KindSource, "id"); err != nil {
			t.Fatalf("Get: %v", err)
		}
	}
	if fet.srcCalls != 1 {
		t.Fatalf("expected 1 upstream call, got %d", fet.srcCalls)
	}
	if got := enc.calls.Load(); got != 1 {
		t.Fatalf("expected 1 encode call, got %d", got)
	}
}

func TestCache_Get_RefreshesAfterStale(t *testing.T) {
	fet := &fakeFetcher{srcFrame: makeFrame("hi")}
	enc := &fakeEncoder{}
	c := NewCache(Config{StaleAfter: 10 * time.Millisecond}, fet, enc)

	if _, err := c.Get(context.Background(), KindSource, "id"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := c.Get(context.Background(), KindSource, "id"); err != nil {
		t.Fatal(err)
	}
	if fet.srcCalls != 2 {
		t.Fatalf("srcCalls=%d", fet.srcCalls)
	}
}

func TestCache_Get_ConcurrentCoalescing(t *testing.T) {
	fet := &fakeFetcher{srcFrame: makeFrame("hi"), delay: 50 * time.Millisecond}
	enc := &fakeEncoder{}
	c := NewCache(Config{StaleAfter: time.Second}, fet, enc)

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for range N {
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), KindSource, "id"); err != nil {
				t.Errorf("Get: %v", err)
			}
		}()
	}
	wg.Wait()
	if fet.srcCalls != 1 {
		t.Fatalf("expected coalesced 1 upstream call, got %d", fet.srcCalls)
	}
}

func TestCache_Get_PropagatesFetcherError(t *testing.T) {
	wantErr := errors.New("upstream down")
	fet := &fakeFetcher{srcErr: wantErr}
	c := NewCache(Config{}, fet, &fakeEncoder{})
	_, err := c.Get(context.Background(), KindSource, "id")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v want %v", err, wantErr)
	}
}

func TestCache_Get_NoFrameYet(t *testing.T) {
	fet := &fakeFetcher{srcFrame: Frame{}} // empty bytes -> ErrNoFrame
	c := NewCache(Config{}, fet, &fakeEncoder{})
	_, err := c.Get(context.Background(), KindSource, "id")
	if !errors.Is(err, ErrNoFrame) {
		t.Fatalf("err=%v want ErrNoFrame", err)
	}
}

func TestSnapshotHandler_ServesJPEGAndETag(t *testing.T) {
	fet := &fakeFetcher{srcFrame: makeFrame("hi")}
	c := NewCache(Config{StaleAfter: time.Second}, fet, &fakeEncoder{})
	mux := http.NewServeMux()
	RegisterAPI(mux, c)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/sources/id/snapshot.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("ct=%q", resp.Header.Get("Content-Type"))
	}
	etag := resp.Header.Get("ETag")
	if !strings.HasPrefix(etag, `"frame-`) {
		t.Fatalf("etag=%q", etag)
	}

	// Replay with If-None-Match → 304
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sources/id/snapshot.jpg", nil)
	req.Header.Set("If-None-Match", etag)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", resp2.StatusCode)
	}
}

func TestParseFPS_ClampingViaConfig(t *testing.T) {
	cfg := Config{MaxFPS: 10, DefaultFPS: 1}
	cfg.fillDefaults()
	cases := []struct {
		in, want int
	}{
		{0, 1},
		{-5, 1},
		{1, 1},
		{5, 5},
		{10, 10},
		{99, 10},
	}
	for _, tc := range cases {
		got := cfg.ClampFPS(tc.in)
		if got != tc.want {
			t.Errorf("ClampFPS(%d)=%d want %d", tc.in, got, tc.want)
		}
	}
}

func TestPreviewHandler_Multipart(t *testing.T) {
	fet := &fakeFetcher{srcFrame: makeFrame("hi")}
	c := NewCache(Config{StaleAfter: 10 * time.Millisecond, MaxFPS: 20, DefaultFPS: 5}, fet, &fakeEncoder{})
	mux := http.NewServeMux()
	RegisterAPI(mux, c)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/sources/id/preview.mjpg?fps=10", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "multipart/x-mixed-replace") {
		t.Fatalf("ct=%q", ct)
	}

	// Read enough bytes to confirm at least 3 JPEG parts arrive in 1.5s.
	// We rely on the request context (deadline above) to cancel the read.
	buf := make([]byte, 16<<10)
	var collected []byte
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			collected = append(collected, buf[:n]...)
		}
		if strings.Count(string(collected), "Content-Type: image/jpeg") >= 3 {
			break
		}
		if err != nil {
			break
		}
	}
	if got := strings.Count(string(collected), "Content-Type: image/jpeg"); got < 3 {
		t.Fatalf("expected ≥3 jpeg parts, got %d (body=%q)", got, collected)
	}
}
