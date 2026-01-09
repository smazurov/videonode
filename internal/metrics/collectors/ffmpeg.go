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
}

// NewFFmpegCollector creates a new FFmpeg collector.
func NewFFmpegCollector(socketPath, streamID string) *FFmpegCollector {
	return &FFmpegCollector{
		logger:     slog.With("component", "ffmpeg_collector", "stream_id", streamID),
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
	})
	return stopErr
}

func (f *FFmpegCollector) startSocketListener() {
	f.logger.Info("Starting socket listener", "socket", f.socketPath)

	if err := os.Remove(f.socketPath); err != nil && !os.IsNotExist(err) {
		f.logger.Warn("Failed to clean up old socket file", "error", err)
	}

	listener, err := net.Listen("unix", f.socketPath)
	if err != nil {
		f.logger.Error("Failed to create Unix socket listener", "error", err)
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
				f.logger.Warn("Error accepting connection", "error", acceptErr)
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
	if fps, err := strconv.ParseFloat(data["fps"], 64); err == nil {
		metrics.SetFFmpegFPS(f.streamID, fps)
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
}
