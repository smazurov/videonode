package recording

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streaming"
)

const (
	snapshotTimeout = 5 * time.Second
	ffmpegTimeout   = 5 * time.Second
)

// SnapshotKind identifies which entity owns a snapshot on disk.
// Files live at <baseDir>/<kind>/<id>/<timestamp>.jpg.
type SnapshotKind string

// Supported snapshot kinds.
const (
	SnapshotKindSource SnapshotKind = "sources"
	SnapshotKindStream SnapshotKind = "streams"
)

// SnapshotStream captures a single JPEG frame from a running stream via its
// RTSP keyframe path and writes it to disk under SnapshotKindStream.
// Returns the path relative to baseDir (e.g. "streams/test/20260404_005015.jpg").
func SnapshotStream(stream *streaming.Stream, baseDir string, timeout time.Duration) (string, error) {
	keyframe, err := CaptureKeyframe(stream, timeout)
	if err != nil {
		return "", err
	}

	jpegData, err := decodeToJPEG(keyframe.Data, keyframe.Codec)
	if err != nil {
		return "", err
	}

	return writeSnapshotFile(jpegData, SnapshotKindStream, stream.ID(), baseDir)
}

// writeSnapshotFile writes JPEG bytes under <baseDir>/<kind>/<id>/<timestamp>.jpg
// and returns the path relative to baseDir.
func writeSnapshotFile(jpeg []byte, kind SnapshotKind, id, baseDir string) (string, error) {
	logger := logging.GetLogger("recording")

	dir := filepath.Join(baseDir, string(kind), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}

	filename := time.Now().Format("20060102_150405") + ".jpg"
	absPath := filepath.Join(dir, filename)
	if err := os.WriteFile(absPath, jpeg, 0o644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}

	relPath := filepath.Join(string(kind), id, filename)
	logger.Debug("Snapshot written", "kind", string(kind), "id", id, "path", absPath, "bytes", len(jpeg))
	return relPath, nil
}

// decodeToJPEG pipes Annex B video data through FFmpeg to produce a JPEG image.
func decodeToJPEG(annexB []byte, codec CodecType) ([]byte, error) {
	logger := logging.GetLogger("recording")

	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-f", string(codec),
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	)

	cmd.Stdin = bytes.NewReader(annexB)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("FFmpeg snapshot decode failed", "error", err, "stderr", stderr.String())
		return nil, fmt.Errorf("ffmpeg decode failed: %w: %s", err, stderr.String())
	}

	if stderr.Len() > 0 {
		logger.Debug("FFmpeg snapshot stderr", "output", stderr.String())
	}

	return stdout.Bytes(), nil
}
