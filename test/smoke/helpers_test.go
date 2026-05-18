//go:build smoke

package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func newReq(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, baseURL+path, rdr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func newAuthReq(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	req := newReq(t, method, path, body)
	req.SetBasicAuth(authUser, authPass)
	return req
}

// doExpect runs the request and asserts the status code. The response body
// is fully drained and closed before returning — callers receive the body
// bytes only.
func doExpect(t *testing.T, req *http.Request, wantStatus int) []byte {
	t.Helper()
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: got status %d, want %d\nbody: %s",
			req.Method, req.URL.Path, resp.StatusCode, wantStatus, body)
	}
	return body
}

func decodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode JSON: %v\nbody: %s", err, body)
	}
}

func waitHealthy(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Accept", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// probeCodec runs ffprobe on a single packet of the given URL and returns
// the codec_name of the first video stream. MPEG-TS over SRT can advertise
// multiple programs, so csv output may span several lines — we return the
// first non-empty trimmed line.
func probeCodec(t *testing.T, ctx context.Context, url string, extraArgs ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return "", fmt.Errorf("ffprobe not in PATH: %w", err)
	}
	args := make([]string, 0, 11+len(extraArgs))
	args = append(args,
		"-v", "error",
		"-read_intervals", "%+#1",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "csv=p=0",
	)
	args = append(args, extraArgs...)
	args = append(args, url)

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffprobe %s: %w\nstderr: %s", url, err, stderr.String())
	}
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s, nil
		}
	}
	return "", nil
}

// dumpServerLogTail prints the tail of the captured server log. Useful when
// a pipeline step fails — gives the agent something to look at.
func dumpServerLogTail(t *testing.T, n int) {
	t.Helper()
	if srvLog == nil {
		return
	}
	lines := strings.Split(srvLog.String(), "\n")
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}
	t.Logf("--- server log tail (last %d lines) ---\n%s\n--- end server log ---",
		len(lines)-start, strings.Join(lines[start:], "\n"))
}

func requireFfprobe(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Fatalf("ffprobe not in PATH; install ffmpeg")
	}
}
