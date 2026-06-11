package streaming

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// Recording lifecycle errors.
var (
	// ErrAlreadyRecording is returned when a stream already has an active recording.
	ErrAlreadyRecording = errors.New("stream is already recording")
	// ErrNotRecording is returned when stopping a stream that isn't recording.
	ErrNotRecording = errors.New("stream is not recording")
	// ErrRecordingNotFound is returned when a recording session doesn't exist on disk.
	ErrRecordingNotFound = errors.New("recording not found")
	// ErrRecordingActive is returned when deleting a session that's still recording.
	ErrRecordingActive = errors.New("recording is active; stop it before deleting")
)

// RecordingConfig is the daemon-wide recording configuration.
type RecordingConfig struct {
	Dir            string // base output directory
	SegmentSec     int    // target HLS segment length
	ThumbnailSec   int    // storyboard interval (0 disables thumbnails)
	ThumbnailWidth int    // storyboard frame width in px
}

// RecordingInfo is a point-in-time view of one recording session.
type RecordingInfo struct {
	RecordingID string    // session id (== Session dir name)
	StreamID    string    // recorded stream
	Session     string    // session dir name (timestamp), used to build playback URLs
	Active      bool      // currently capturing
	StartedAt   time.Time // UTC start
	Segments    int       // HLS segments written so far
}

// ThumbnailSource fetches a fresh JPEG of an upstream entity (kind is
// "sources"/"composers"). Backed by the snapshot cache in main.go.
type ThumbnailSource func(ctx context.Context, kind, id string) (jpeg []byte, width, height int, err error)

// RecordingManager owns manual start/stop recordings, at most one per stream.
// It mirrors SRTServer's consumer registry but writes fMP4/HLS to disk.
type RecordingManager struct {
	streams  StreamProvider
	cfg      RecordingConfig
	thumbSrc ThumbnailSource // optional; nil disables the thumbnail track
	logger   *slog.Logger

	mu     sync.Mutex
	active map[string]*RecordingConsumer // streamID -> consumer
}

// NewRecordingManager constructs a manager. A nil thumbSrc disables storyboard
// thumbnails.
func NewRecordingManager(streams StreamProvider, cfg RecordingConfig, thumbSrc ThumbnailSource, logger *slog.Logger) *RecordingManager {
	if cfg.Dir == "" {
		cfg.Dir = "./recordings"
	}
	return &RecordingManager{
		streams:  streams,
		cfg:      cfg,
		thumbSrc: thumbSrc,
		logger:   logger,
		active:   make(map[string]*RecordingConsumer),
	}
}

// Dir returns the configured base recordings directory.
func (m *RecordingManager) Dir() string { return m.cfg.Dir }

// Start begins recording streamID. The upstreamRef ("source:<id>" /
// "composer:<id>") drives the storyboard thumbnail track. It lazily spawns the
// encoder (and pins it up for the recording's duration) via EnsureStreamReady.
func (m *RecordingManager) Start(streamID, upstreamRef string) (RecordingInfo, error) {
	m.mu.Lock()
	if _, ok := m.active[streamID]; ok {
		m.mu.Unlock()
		return RecordingInfo{}, ErrAlreadyRecording
	}
	m.mu.Unlock()

	stream := m.streams.EnsureStreamReady(streamID, 3*time.Second)
	if stream == nil {
		return RecordingInfo{}, ErrStreamNotFound
	}

	session := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(m.cfg.Dir, streamID, session)

	thumbCfg := thumbnailConfig{
		intervalSec: m.cfg.ThumbnailSec,
		width:       m.cfg.ThumbnailWidth,
	}
	if m.thumbSrc != nil && m.cfg.ThumbnailSec > 0 {
		if kind, id, ok := parseUpstreamRef(upstreamRef); ok {
			thumbCfg.fetch = func(ctx context.Context) ([]byte, int, int, error) {
				return m.thumbSrc(ctx, kind, id)
			}
		}
	}

	consumer, err := newRecordingConsumer(stream, session, dir, m.cfg.SegmentSec, thumbCfg, m.logger)
	if err != nil {
		return RecordingInfo{}, err
	}

	m.mu.Lock()
	// Lost a race: another Start won. Tear ours down.
	if _, ok := m.active[streamID]; ok {
		m.mu.Unlock()
		_ = consumer.Stop()
		return RecordingInfo{}, ErrAlreadyRecording
	}
	m.active[streamID] = consumer
	m.mu.Unlock()

	m.logger.Info("recording started",
		logging.KeyStreamID, streamID, logging.KeySession, session, logging.KeyPath, dir)
	return consumer.info(), nil
}

// Stop finalizes the active recording for streamID.
func (m *RecordingManager) Stop(streamID string) (RecordingInfo, error) {
	m.mu.Lock()
	consumer, ok := m.active[streamID]
	if ok {
		delete(m.active, streamID)
	}
	m.mu.Unlock()
	if !ok {
		return RecordingInfo{}, ErrNotRecording
	}
	info := consumer.info()
	info.Active = false
	_ = consumer.Stop()
	m.logger.Info("recording stopped", logging.KeyStreamID, streamID, logging.KeySession, info.Session)
	return info, nil
}

// Status returns the active recording for streamID, if any.
func (m *RecordingManager) Status(streamID string) (RecordingInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.active[streamID]; ok {
		return c.info(), true
	}
	return RecordingInfo{}, false
}

// List returns all active recordings.
func (m *RecordingManager) List() []RecordingInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RecordingInfo, 0, len(m.active))
	for _, c := range m.active {
		out = append(out, c.info())
	}
	return out
}

// DeleteSession removes a completed recording session's files from disk. It
// refuses to delete the session that's currently recording (stop it first) and
// guards against path traversal in streamID/session.
func (m *RecordingManager) DeleteSession(streamID, session string) error {
	if streamID == "" || session == "" {
		return ErrRecordingNotFound
	}

	m.mu.Lock()
	c, ok := m.active[streamID]
	isActive := ok && c.recordingID == session
	m.mu.Unlock()
	if isActive {
		return ErrRecordingActive
	}

	root, err := filepath.Abs(m.cfg.Dir)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, streamID, session)
	// Confine to <root>/<stream>/<session>: reject any traversal.
	if filepath.Dir(filepath.Dir(dir)) != root {
		return ErrRecordingNotFound
	}
	if _, statErr := os.Stat(filepath.Join(dir, "index.m3u8")); statErr != nil {
		return ErrRecordingNotFound
	}
	return os.RemoveAll(dir)
}

// CloseStreamConsumers finalizes any active recording for a stream when its
// producer is replaced/removed, so the on-disk HLS is closed cleanly.
func (m *RecordingManager) CloseStreamConsumers(streamID string) {
	m.mu.Lock()
	consumer, ok := m.active[streamID]
	if ok {
		delete(m.active, streamID)
	}
	m.mu.Unlock()
	if ok {
		m.logger.Info("recording finalized on producer replace", logging.KeyStreamID, streamID)
		_ = consumer.Stop()
	}
}

// StopAll finalizes every active recording (daemon shutdown).
func (m *RecordingManager) StopAll() {
	m.mu.Lock()
	consumers := make([]*RecordingConsumer, 0, len(m.active))
	for id, c := range m.active {
		consumers = append(consumers, c)
		delete(m.active, id)
	}
	m.mu.Unlock()
	for _, c := range consumers {
		_ = c.Stop()
	}
}

// parseUpstreamRef splits "source:<id>"/"composer:<id>" into a snapshot kind
// ("sources"/"composers") and id.
func parseUpstreamRef(ref string) (kind, id string, ok bool) {
	prefix, rest, found := strings.Cut(ref, ":")
	if !found || rest == "" {
		return "", "", false
	}
	switch prefix {
	case "source":
		return "sources", rest, true
	case "composer":
		return "composers", rest, true
	default:
		return "", "", false
	}
}
