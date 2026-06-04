// Package collectors provides metrics collectors for FFmpeg and MPP.
package collectors

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics"
)

// FFmpegCollector collects FFmpeg progress data via Unix socket.
type FFmpegCollector struct {
	logger     *slog.Logger
	socketPath string
	streamID   string
	listener   net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	stopOnce   sync.Once

	fpsMu sync.Mutex
	fps   fpsTracker
}

// fpsEMAAlpha controls how fast the smoothed FPS tracks the instantaneous
// rate. With FFmpeg's ~0.5s progress cadence this gives an effective window of
// roughly 1.5s — responsive to real changes, but not jumpy.
const fpsEMAAlpha = 0.4

// fpsSanityCeiling discards obviously bogus instantaneous samples (e.g. when
// the FFmpeg frame counter resets after a restart and produces a huge delta).
const fpsSanityCeiling = 1000.0

// fpsTracker derives a smoothed frames-per-second from successive
// (frameCounter, wallTime) samples. FFmpeg's own progress fps is a cumulative
// average since stream start, which becomes effectively frozen for
// long-running streams; deriving from frame counter deltas reflects current
// throughput.
type fpsTracker struct {
	lastFrame uint64
	lastTime  time.Time
	smoothed  float64
	seeded    bool
}

// update feeds a new (frame, now) sample and returns the smoothed fps along
// with a bool indicating whether a value is available yet. On the first
// sample, or after a counter reset, it returns (0, false) — the caller should
// leave the existing metric untouched in that case.
func (t *fpsTracker) update(frame uint64, now time.Time) (float64, bool) {
	prevFrame, prevTime := t.lastFrame, t.lastTime
	t.lastFrame, t.lastTime = frame, now

	if prevTime.IsZero() {
		return 0, false
	}
	dt := now.Sub(prevTime).Seconds()
	if dt <= 0 || frame < prevFrame {
		// Counter reset (e.g. ffmpeg restart) or clock jitter — re-base.
		t.seeded = false
		return 0, false
	}

	instant := float64(frame-prevFrame) / dt
	if instant < 0 || instant > fpsSanityCeiling {
		return 0, false
	}

	if !t.seeded {
		t.smoothed = instant
		t.seeded = true
	} else {
		t.smoothed = fpsEMAAlpha*instant + (1-fpsEMAAlpha)*t.smoothed
	}
	return t.smoothed, true
}

// reset clears the tracker so the next update re-seeds from scratch.
func (t *fpsTracker) reset() {
	*t = fpsTracker{}
}

// NewFFmpegCollector creates a new FFmpeg collector.
func NewFFmpegCollector(socketPath, streamID string) *FFmpegCollector {
	return &FFmpegCollector{
		logger:     logging.GetLogger("streams").With(logging.KeyStreamID, streamID),
		socketPath: socketPath,
		streamID:   streamID,
	}
}

// Start begins collecting FFmpeg data.
func (f *FFmpegCollector) Start(ctx context.Context) error {
	f.ctx, f.cancel = context.WithCancel(ctx)
	go f.startSocketListener()
	return nil
}

// Stop stops the FFmpeg collector.
func (f *FFmpegCollector) Stop() error {
	var stopErr error
	f.stopOnce.Do(func() {
		if f.cancel != nil {
			f.cancel()
		}
		if f.listener != nil {
			f.listener.Close()
			f.listener = nil
		}
		if f.socketPath != "" {
			os.Remove(f.socketPath)
		}
		metrics.DeleteFFmpegMetrics(f.streamID)
		f.fpsMu.Lock()
		f.fps.reset()
		f.fpsMu.Unlock()
	})
	return stopErr
}

func (f *FFmpegCollector) startSocketListener() {
	f.logger.Info("Starting socket listener", logging.KeySocket, f.socketPath)

	if err := os.Remove(f.socketPath); err != nil && !os.IsNotExist(err) {
		f.logger.Warn("Failed to clean up old socket file", logging.KeyError, err)
	}

	listener, err := net.Listen("unix", f.socketPath)
	if err != nil {
		f.logger.Error("Failed to create Unix socket listener", logging.KeyError, err)
		return
	}

	f.listener = listener
	defer func() {
		listener.Close()
		os.Remove(f.socketPath)
	}()

	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		if ul, ok := listener.(*net.UnixListener); ok {
			ul.SetDeadline(time.Now().Add(1 * time.Second))
		}

		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			var netErr net.Error
			if errors.As(acceptErr, &netErr) {
				continue
			}
			select {
			case <-f.ctx.Done():
				return
			default:
				if strings.Contains(acceptErr.Error(), "use of closed network connection") {
					return
				}
				f.logger.Warn("Error accepting connection", logging.KeyError, acceptErr)
				continue
			}
		}

		go f.handleConnection(conn)
	}
}

func (f *FFmpegCollector) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	progressData := make(map[string]string)

	for scanner.Scan() {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				progressData[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "progress=") {
			f.sendProgressMetrics(progressData)
			progressData = make(map[string]string)
		}
	}
}

func (f *FFmpegCollector) sendProgressMetrics(data map[string]string) {
	if !f.updateFPS(data) {
		// Fall back to ffmpeg's reported (cumulative-average) fps if we can't
		// parse the frame counter — better a lagging value than no value.
		if fps, err := strconv.ParseFloat(data["fps"], 64); err == nil {
			metrics.SetFFmpegFPS(f.streamID, fps)
		}
	}
	if dropped, err := strconv.ParseFloat(data["drop_frames"], 64); err == nil {
		metrics.SetFFmpegDroppedFrames(f.streamID, dropped)
	}
	if dup, err := strconv.ParseFloat(data["dup_frames"], 64); err == nil {
		metrics.SetFFmpegDuplicateFrames(f.streamID, dup)
	}
	speedStr := strings.TrimSuffix(data["speed"], "x")
	if speed, err := strconv.ParseFloat(strings.TrimSpace(speedStr), 64); err == nil {
		metrics.SetFFmpegSpeed(f.streamID, speed)
	}

	// FFmpeg emits `progress=end` on the final block of a session. Reset so
	// the next session re-seeds the EMA from its own first sample.
	if data["progress"] == "end" {
		f.fpsMu.Lock()
		f.fps.reset()
		f.fpsMu.Unlock()
	}
}

// updateFPS computes a delta-based smoothed FPS from the progress block's
// `frame` counter and publishes it to the metrics registry. Returns true if a
// value was published, false if the caller should fall back to ffmpeg's
// reported fps (e.g. missing/unparseable frame field, or first sample).
func (f *FFmpegCollector) updateFPS(data map[string]string) bool {
	frame, err := strconv.ParseUint(data["frame"], 10, 64)
	if err != nil {
		return false
	}
	f.fpsMu.Lock()
	smoothed, ok := f.fps.update(frame, time.Now())
	f.fpsMu.Unlock()
	if !ok {
		return false
	}
	metrics.SetFFmpegFPS(f.streamID, smoothed)
	return true
}
