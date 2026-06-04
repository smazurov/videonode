//go:build smoke

package smoke

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Package-level handles shared across all tests in this suite.
var (
	binPath   string
	runDir    string
	repoRoot  string
	baseURL   string
	httpPort  int
	rtspPort  int
	srtPort   int
	authUser  = "smoke"
	authPass  = "smoke"
	srvCmd    *exec.Cmd
	srvLog    *bytes.Buffer
	srvCancel context.CancelFunc

	// Set by main_test once detection runs, consumed by encoder tests.
	expectedEncoderFamily string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	var err error
	runDir, err = os.MkdirTemp("", "videonode-smoke-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdirtemp:", err)
		return 1
	}

	keepDir := false
	defer func() {
		if keepDir {
			fmt.Fprintf(os.Stderr, "\nsmoke run dir kept at: %s\n", runDir)
			return
		}
		_ = os.RemoveAll(runDir)
	}()

	// Resolve repo root. The cwd while running `go test ./test/smoke/...`
	// from the repo root is the test package directory: <repo>/test/smoke.
	// Walk up to find go.mod.
	repoRoot, err = findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "find repo root:", err)
		keepDir = true
		return 1
	}
	fmt.Fprintln(os.Stderr, "smoke: repo root =", repoRoot)
	fmt.Fprintln(os.Stderr, "smoke: run dir   =", runDir)

	// 1. Build the binary.
	binPath = filepath.Join(runDir, "videonode")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "go build failed: %v\n%s\n", err, out)
		keepDir = true
		return 1
	}
	fmt.Fprintln(os.Stderr, "smoke: built binary at", binPath)

	// 2. Allocate ports.
	// HTTP and SRT can be ephemeral, but RTSP must be 8554 because the
	// FFmpeg pipeline that publishes to the internal RTSP server has the
	// output URL hardcoded at internal/streams/processor.go:102 and
	// internal/streams/canvas_processor.go:177 (rtsp://127.0.0.1:8554/<id>).
	// Pre-flight check :8554 so we fail fast with a clear message if it's
	// in use.
	httpPort = mustFreePortNoT()
	rtspPort = 8554
	srtPort = mustFreePortNoT()
	if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", rtspPort)); err != nil {
		fmt.Fprintf(os.Stderr, "smoke: RTSP port %d is in use — another videonode (air?) is likely running. Stop it before running smoke tests.\n", rtspPort)
		keepDir = true
		return 1
	} else {
		_ = ln.Close()
	}
	baseURL = fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	fmt.Fprintf(os.Stderr, "smoke: ports http=%d rtsp=%d srt=%d\n", httpPort, rtspPort, srtPort)

	// 3. Run validate-encoders in runDir to bootstrap streams.toml with a
	//    [validation.*] block. The subcommand is lightweight (no server
	//    init runs), and writing into an empty runDir keeps the bootstrap
	//    streams out of the picture until step 4 appends them.
	val := exec.Command(binPath, "validate-encoders", "-q")
	val.Dir = runDir
	val.Stdout = os.Stderr
	val.Stderr = os.Stderr
	fmt.Fprintln(os.Stderr, "smoke: running validate-encoders...")
	if err := val.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "validate-encoders failed: %v\n", err)
		keepDir = true
		return 1
	}

	// 4. Append bootstrap streams to the streams.toml that
	//    validate-encoders just wrote — preserving its [validation.*]
	//    section so TestEncoderFamily can parse it.
	bootstrapConfig := filepath.Join(repoRoot, "test", "smoke", "testdata", "streams.smoke.toml")
	if err := appendStreamsToConfig(filepath.Join(runDir, "streams.toml"), bootstrapConfig); err != nil {
		fmt.Fprintln(os.Stderr, "append bootstrap streams:", err)
		keepDir = true
		return 1
	}

	// 5. Detect expected encoder family (used by TestEncoderFamily).
	expectedEncoderFamily = detectExpectedEncoderFamily()
	fmt.Fprintln(os.Stderr, "smoke: expected encoder family =", expectedEncoderFamily)

	// 6. Spawn the server.
	ctx, cancel := context.WithCancel(context.Background())
	srvCancel = cancel
	srvCmd = exec.CommandContext(ctx, binPath)
	srvCmd.Dir = runDir
	// Env vars are namespaced as VIDEONODE_<env_tag> by
	// internal/config/config.go:83 — the bare names from the struct tags
	// won't be picked up.
	srvCmd.Env = append(os.Environ(),
		"VIDEONODE_AUTH_TYPE=basic",
		"VIDEONODE_AUTH_USERNAME="+authUser,
		"VIDEONODE_AUTH_PASSWORD="+authPass,
		fmt.Sprintf("VIDEONODE_SERVER_PORT=:%d", httpPort),
		fmt.Sprintf("VIDEONODE_STREAMING_RTSP_PORT=:%d", rtspPort),
		fmt.Sprintf("VIDEONODE_SRT_ADDR=:%d", srtPort),
		"VIDEONODE_SRT_ENABLED=true",
		"VIDEONODE_STREAMS_CONFIG_FILE="+filepath.Join(runDir, "streams.toml"),
		"VIDEONODE_RECORDING_DATA_DIR="+filepath.Join(runDir, "recording"),
		"VIDEONODE_FEATURES_LED_CONTROL=false",
		"VIDEONODE_UPDATE_ENABLED=false",
		"VIDEONODE_METRICS_SSE_ENABLED=true",
	)
	srvCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	srvCmd.Cancel = func() error {
		// Negative PID signals the whole process group, which includes any
		// FFmpeg children spawned by the stream service.
		return syscall.Kill(-srvCmd.Process.Pid, syscall.SIGTERM)
	}
	srvCmd.WaitDelay = 5 * time.Second
	srvLog = &bytes.Buffer{}
	logFile, err := os.Create(filepath.Join(runDir, "server.log"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "create server log:", err)
		keepDir = true
		cancel()
		return 1
	}
	defer logFile.Close()
	srvCmd.Stdout = io.MultiWriter(srvLog, logFile)
	srvCmd.Stderr = io.MultiWriter(srvLog, logFile)
	if err := srvCmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start videonode:", err)
		keepDir = true
		cancel()
		return 1
	}
	fmt.Fprintf(os.Stderr, "smoke: server started pid=%d\n", srvCmd.Process.Pid)

	// 7. Wait for /api/health.
	if !waitHealthy(baseURL+"/api/health", 30*time.Second) {
		fmt.Fprintln(os.Stderr, "server never became healthy")
		dumpServerLogTailToStderr(200)
		cancel()
		_ = srvCmd.Wait()
		keepDir = true
		return 1
	}
	fmt.Fprintln(os.Stderr, "smoke: server is healthy")

	// 8. Run the tests.
	code := m.Run()

	// 9. Teardown.
	fmt.Fprintln(os.Stderr, "smoke: shutting down server...")
	cancel()
	_ = srvCmd.Wait()
	if code != 0 {
		keepDir = true
		dumpServerLogTailToStderr(200)
	}
	return code
}

// mustFreePortNoT is the package-init variant that has no *testing.T.
func mustFreePortNoT() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found walking up from cwd")
		}
		dir = parent
	}
}

// appendStreamsToConfig appends the [streams.*] blocks from bootstrap to
// the existing config file, skipping bootstrap's `version = ...` line
// since the existing file already has one.
func appendStreamsToConfig(existing, bootstrap string) error {
	bootstrapData, err := os.ReadFile(bootstrap)
	if err != nil {
		return fmt.Errorf("read bootstrap: %w", err)
	}

	out, err := os.OpenFile(existing, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open existing for append: %w", err)
	}
	defer out.Close()

	if _, err := out.WriteString("\n"); err != nil {
		return err
	}
	for line := range strings.SplitSeq(string(bootstrapData), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "version") && strings.Contains(trimmed, "=") {
			continue
		}
		if _, err := out.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func dumpServerLogTailToStderr(n int) {
	if srvLog == nil {
		return
	}
	s := srvLog.String()
	// Print last n lines.
	idx := len(s)
	lines := 0
	for i := len(s) - 1; i >= 0 && lines <= n; i-- {
		if s[i] == '\n' {
			lines++
			idx = i + 1
		}
	}
	fmt.Fprintf(os.Stderr, "\n--- server log tail (last %d lines) ---\n%s\n--- end server log ---\n", n, s[idx:])
}
