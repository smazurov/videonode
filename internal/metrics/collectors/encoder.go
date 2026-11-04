// Package collectors provides metrics collectors for the encoder stage.
package collectors

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics"
)

// EncoderCollector collects encoder-stage status on one per-stream Unix
// socket: ffmpeg's -progress reports and vn-sink's status reports. Each
// connection is routed by its first line; only a sink_wire hello marks a
// vn-sink connection, everything else is parsed as ffmpeg progress.
type EncoderCollector struct {
	logger     *slog.Logger
	socketPath string
	streamID   string
	listener   net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	stopOnce   sync.Once

	mu         sync.Mutex
	fps        fpsTracker
	progress   metrics.Activity
	frames     metrics.Activity
	stats      metrics.FFmpegStreamMetrics
	sink       sinkState
	generation uint64
	runID      int64
	conns      map[net.Conn]struct{}
	stopped    bool
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

// NewEncoderCollector creates a new FFmpeg collector.
func NewEncoderCollector(socketPath, streamID string) *EncoderCollector {
	return &EncoderCollector{
		logger:     logging.GetLogger("streams").With(logging.KeyStreamID, streamID),
		socketPath: socketPath,
		streamID:   streamID,
		conns:      make(map[net.Conn]struct{}),
	}
}

// Start binds the progress socket before returning so encoders can connect immediately.
func (f *EncoderCollector) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return errors.New("encoder collector is stopped")
	}
	if f.listener != nil {
		return errors.New("encoder collector is already started")
	}
	if err := os.Remove(f.socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", f.socketPath)
	if err != nil {
		return err
	}
	f.listener = listener
	f.logger.Debug("Listening for encoder progress", logging.KeySocket, f.socketPath)
	f.ctx, f.cancel = context.WithCancel(ctx)
	go f.acceptConnections(listener)
	context.AfterFunc(f.ctx, func() { _ = f.Stop() })
	return nil
}

// Stop closes the listener and active progress readers, and removes metrics.
func (f *EncoderCollector) Stop() error {
	f.stopOnce.Do(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.stopped = true
		if f.cancel != nil {
			f.cancel()
		}
		if f.listener != nil {
			f.listener.Close()
		}
		for conn := range f.conns {
			conn.Close()
		}
		metrics.DeleteFFmpegMetrics(f.streamID)
		metrics.DeleteSinkMetrics(f.streamID)
		f.reset()
	})
	return nil
}

// BeginRun discards observations and connections belonging to the previous encoder.
func (f *EncoderCollector) BeginRun(runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return errors.New("encoder collector is stopped")
	}
	f.runID = runID
	f.generation++
	for conn := range f.conns {
		conn.Close()
	}
	f.reset()
	metrics.DeleteFFmpegMetrics(f.streamID)
	metrics.DeleteSinkMetrics(f.streamID)
	// Rebind to discard queued connections from the old run, not just accepted ones.
	if f.listener != nil {
		f.listener.Close()
		f.listener = nil
	}
	if f.ctx != nil {
		listener, err := net.Listen("unix", f.socketPath)
		if err != nil {
			return err
		}
		f.listener = listener
		go f.acceptConnections(listener)
	}
	return nil
}

func (f *EncoderCollector) reset() {
	f.fps.reset()
	f.progress = metrics.Activity{}
	f.frames = metrics.Activity{}
	f.stats = metrics.FFmpegStreamMetrics{}
	f.sink = sinkState{}
}

// Health returns the last reported values with report and frame-advance freshness.
func (f *EncoderCollector) Health(runID int64, now time.Time) metrics.FFmpegHealth {
	f.mu.Lock()
	defer f.mu.Unlock()
	if runID != f.runID {
		empty := metrics.Activity{}.Snapshot(now)
		return metrics.FFmpegHealth{Progress: empty, Frames: empty}
	}
	return metrics.FFmpegHealth{
		Progress: f.progress.Snapshot(now),
		Frames:   f.frames.Snapshot(now),
		Stats:    f.stats,
	}
}

func (f *EncoderCollector) acceptConnections(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		if f.stopped || listener != f.listener {
			f.mu.Unlock()
			conn.Close()
			return
		}
		generation := f.generation
		f.conns[conn] = struct{}{}
		f.mu.Unlock()
		go f.handleConnection(conn, generation)
	}
}

func (f *EncoderCollector) handleConnection(conn net.Conn, generation uint64) {
	defer func() {
		conn.Close()
		f.mu.Lock()
		delete(f.conns, conn)
		f.mu.Unlock()
	}()

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}
	first := strings.TrimSpace(scanner.Text())
	if version, ok := strings.CutPrefix(first, sinkHelloKey+"="); ok {
		if version != sinkWireVersion {
			f.logger.Warn("Rejecting vn-sink status connection with unsupported wire version", logging.KeyVersion, version)
			return
		}
		readBlocks(scanner, "", "status", func(data map[string]string) {
			f.applySinkStatus(data, generation, time.Now())
		})
		return
	}
	readBlocks(scanner, first, "progress", func(data map[string]string) {
		f.sendProgressMetrics(data, generation, time.Now())
	})
}

// readBlocks feeds key=value lines into blocks closed by the terminator key.
func readBlocks(scanner *bufio.Scanner, first, terminator string, onBlock func(map[string]string)) {
	data := make(map[string]string)
	line := first
	for {
		if key, value, ok := strings.Cut(line, "="); ok {
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			data[key] = value
			if key == terminator {
				onBlock(data)
				clear(data)
			}
		}
		if !scanner.Scan() {
			return
		}
		line = strings.TrimSpace(scanner.Text())
	}
}

func (f *EncoderCollector) sendProgressMetrics(data map[string]string, generation uint64, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped || generation != f.generation {
		return
	}
	f.progress.Record(now)
	if frame, err := strconv.ParseUint(data["frame"], 10, 64); err == nil {
		if frame < f.frames.Count {
			f.frames = metrics.Activity{}
		}
		if frame > f.frames.Count {
			f.frames.Last = now
		}
		f.frames.Count = frame
		if fps, ok := f.fps.update(frame, now); ok {
			f.stats.FPS = fps
		} else if reported, valid := parseProgressNumber(data["fps"]); valid {
			f.stats.FPS = reported
		}
	} else if fps, valid := parseProgressNumber(data["fps"]); valid {
		f.stats.FPS = fps
	}
	if dropped, valid := parseProgressNumber(data["drop_frames"]); valid {
		f.stats.DroppedFrames = dropped
	}
	if dup, valid := parseProgressNumber(data["dup_frames"]); valid {
		f.stats.DuplicateFrames = dup
	}
	speed := strings.TrimSpace(strings.TrimSuffix(data["speed"], "x"))
	if value, valid := parseProgressNumber(speed); valid {
		f.stats.Speed = value
	}
	metrics.SetFFmpegFPS(f.streamID, f.stats.FPS)
	metrics.SetFFmpegDroppedFrames(f.streamID, f.stats.DroppedFrames)
	metrics.SetFFmpegDuplicateFrames(f.streamID, f.stats.DuplicateFrames)
	metrics.SetFFmpegSpeed(f.streamID, f.stats.Speed)
	if data["progress"] == "end" {
		f.fps.reset()
	}
}

func parseProgressNumber(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}
