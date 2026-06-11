package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/streaming"
)

const recordingSessionLayout = "20060102T150405Z"

// registerRecordingRoutes registers recording start/stop/status/list endpoints.
// No-op when the recording manager isn't wired.
func (s *Server) registerRecordingRoutes() {
	mgr := s.options.RecordingManager
	if mgr == nil {
		return
	}

	huma.Register(s.api, huma.Operation{
		OperationID: "start-recording",
		Method:      http.MethodPost,
		Path:        "/api/streams/{stream_id}/recording",
		Summary:     "Start Recording",
		Description: "Start recording a stream to fMP4/HLS on disk. Pins the encoder up for the recording's duration.",
		Tags:        []string{"recordings"},
		Errors:      []int{401, 404, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
	},
	) (*models.RecordingResponse, error) {
		upstream := ""
		if s.streamService != nil {
			st, err := s.streamService.Get(ctx, input.StreamID)
			if err != nil {
				return nil, s.mapStreamError(err)
			}
			upstream = st.Upstream
		}
		info, err := mgr.Start(input.StreamID, upstream)
		if err != nil {
			return nil, mapRecordingError(err)
		}
		data := recordingInfoToAPI(mgr.Dir(), info)
		if s.recordingEntity != nil {
			s.recordingEntity.PublishCreated(data)
		}
		return &models.RecordingResponse{Body: data}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "stop-recording",
		Method:      http.MethodDelete,
		Path:        "/api/streams/{stream_id}/recording",
		Summary:     "Stop Recording",
		Description: "Stop the active recording for a stream and finalize the playlist.",
		Tags:        []string{"recordings"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
	},
	) (*models.RecordingResponse, error) {
		info, err := mgr.Stop(input.StreamID)
		if err != nil {
			return nil, mapRecordingError(err)
		}
		data := recordingInfoToAPI(mgr.Dir(), info)
		if s.recordingEntity != nil {
			s.recordingEntity.PublishUpdated(data)
		}
		return &models.RecordingResponse{Body: data}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-recording",
		Method:      http.MethodGet,
		Path:        "/api/streams/{stream_id}/recording",
		Summary:     "Get Recording Status",
		Description: "Get the active recording status for a stream.",
		Tags:        []string{"recordings"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
	},
	) (*models.RecordingResponse, error) {
		info, ok := mgr.Status(input.StreamID)
		if !ok {
			return nil, huma.Error404NotFound("stream is not recording")
		}
		return &models.RecordingResponse{Body: recordingInfoToAPI(mgr.Dir(), info)}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-recording",
		Method:      http.MethodDelete,
		Path:        "/api/streams/{stream_id}/recordings/{session}",
		Summary:     "Delete Recording",
		Description: "Delete a completed recording session's files from disk.",
		Tags:        []string{"recordings"},
		Errors:      []int{401, 404, 409, 500},
		Security:    withAuth(),
	}, func(_ context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
		Session  string `path:"session" example:"20260610T120000Z" doc:"Recording session id"`
	},
	) (*struct{}, error) {
		if err := mgr.DeleteSession(input.StreamID, input.Session); err != nil {
			return nil, mapRecordingError(err)
		}
		if s.recordingEntity != nil {
			s.recordingEntity.PublishDeleted(input.StreamID + "/" + input.Session)
		}
		return &struct{}{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "list-recordings",
		Method:      http.MethodGet,
		Path:        "/api/recordings",
		Summary:     "List Recordings",
		Description: "List recording sessions (active in-memory + completed on disk).",
		Tags:        []string{"recordings"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(_ context.Context, _ *struct{}) (*models.RecordingListResponse, error) {
		activeInfos := mgr.List()
		active := make([]models.RecordingStatusData, len(activeInfos))
		for i, info := range activeInfos {
			active[i] = recordingInfoToAPI(mgr.Dir(), info)
		}
		list := mergeRecordings(active, scanRecordings(mgr.Dir()))
		return &models.RecordingListResponse{
			Body: models.RecordingListData{Recordings: list, Count: len(list)},
		}, nil
	})

	// Recordings finalized outside the API Stop path (producer replaced,
	// write failure, shutdown) still need a terminal SSE event or clients
	// stay stuck on active:true.
	if s.recordingEntity != nil {
		mgr.SetOnFinalized(func(info streaming.RecordingInfo) {
			s.recordingEntity.PublishUpdated(recordingInfoToAPI(mgr.Dir(), info))
		})
	}

	// Push progress (segments/size/duration) for active sessions so clients
	// stay live without polling. No-op while nothing records; ends on Stop.
	if s.recordingEntity != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-s.done:
					return
				case <-ticker.C:
					for _, info := range mgr.List() {
						s.recordingEntity.PublishUpdated(recordingInfoToAPI(mgr.Dir(), info))
					}
				}
			}
		}()
	}
}

// recordingInfoToAPI projects a streaming.RecordingInfo into the API view,
// deriving the playback/thumbnail URLs and on-disk size from the session dir.
func recordingInfoToAPI(baseDir string, info streaming.RecordingInfo) models.RecordingStatusData {
	base := fmt.Sprintf("/api/streams/%s/recordings/%s", info.StreamID, info.RecordingID)
	size := info.SizeBytes
	duration := info.DurationSeconds
	if !info.Active {
		// Completed sessions: read the truth from disk once.
		dir := filepath.Join(baseDir, info.StreamID, info.RecordingID)
		size = sessionSize(dir)
		duration = playlistDuration(dir)
	}
	return models.RecordingStatusData{
		RecordingID:      info.RecordingID,
		StreamID:         info.StreamID,
		Active:           info.Active,
		StartedAt:        info.StartedAt,
		Segments:         info.Segments,
		SizeBytes:        size,
		DurationSeconds:  duration,
		PlaylistURL:      base + "/index.m3u8",
		ThumbnailsVTTURL: base + "/thumbnails.vtt",
	}
}

// playlistDuration sums the #EXTINF segment durations in the session playlist.
func playlistDuration(sessionDir string) float64 {
	data, err := os.ReadFile(filepath.Join(sessionDir, "index.m3u8"))
	if err != nil {
		return 0
	}
	var total float64
	for line := range strings.Lines(string(data)) {
		rest, ok := strings.CutPrefix(line, "#EXTINF:")
		if !ok {
			continue
		}
		if v, perr := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(rest), ","), 64); perr == nil {
			total += v
		}
	}
	return total
}

// sessionSize sums regular-file sizes under a session dir (segments + thumbs).
func sessionSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if fi, ferr := d.Info(); ferr == nil {
			total += fi.Size()
		}
		return nil
	})
	return total
}

// scanRecordings discovers completed sessions on disk under baseDir, shaped as
// <baseDir>/<stream_id>/<session>/index.m3u8.
func scanRecordings(baseDir string) []models.RecordingStatusData {
	streamDirs, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}
	var out []models.RecordingStatusData
	for _, sd := range streamDirs {
		if !sd.IsDir() {
			continue
		}
		streamID := sd.Name()
		sessions, err := os.ReadDir(filepath.Join(baseDir, streamID))
		if err != nil {
			continue
		}
		for _, ses := range sessions {
			if !ses.IsDir() {
				continue
			}
			session := ses.Name()
			if data, ok := diskRecordingData(baseDir, streamID, session); ok {
				out = append(out, data)
			}
		}
	}
	return out
}

// diskRecordingData builds the API view of one on-disk (inactive) session;
// ok is false when the session dir has no playlist.
func diskRecordingData(baseDir, streamID, session string) (models.RecordingStatusData, bool) {
	sessionDir := filepath.Join(baseDir, streamID, session)
	if _, statErr := os.Stat(filepath.Join(sessionDir, "index.m3u8")); statErr != nil {
		return models.RecordingStatusData{}, false
	}
	started, _ := time.Parse(recordingSessionLayout, session)
	return models.RecordingStatusData{
		RecordingID:      session,
		StreamID:         streamID,
		Active:           false,
		StartedAt:        started.UTC(),
		Segments:         countSegments(sessionDir),
		SizeBytes:        sessionSize(sessionDir),
		DurationSeconds:  playlistDuration(sessionDir),
		PlaylistURL:      fmt.Sprintf("/api/streams/%s/recordings/%s/index.m3u8", streamID, session),
		ThumbnailsVTTURL: fmt.Sprintf("/api/streams/%s/recordings/%s/thumbnails.vtt", streamID, session),
	}, true
}

// recordingEventID is the entity-event id: "<stream_id>/<session>".
func recordingEventID(r models.RecordingStatusData) string {
	return r.StreamID + "/" + r.RecordingID
}

// loadRecordingByEventID resolves an entity-event id back to a session view,
// preferring the live manager state over the disk scan.
func loadRecordingByEventID(mgr *streaming.RecordingManager, id string) (models.RecordingStatusData, error) {
	streamID, session, ok := strings.Cut(id, "/")
	if !ok {
		return models.RecordingStatusData{}, fmt.Errorf("malformed recording id %q", id)
	}
	if info, active := mgr.Status(streamID); active && info.RecordingID == session {
		return recordingInfoToAPI(mgr.Dir(), info), nil
	}
	data, found := diskRecordingData(mgr.Dir(), streamID, session)
	if !found {
		return models.RecordingStatusData{}, streaming.ErrRecordingNotFound
	}
	return data, nil
}

func countSegments(sessionDir string) int {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".m4s" {
			n++
		}
	}
	return n
}

// mergeRecordings overlays active sessions (authoritative) onto the disk scan,
// keyed by stream_id+session, newest first.
func mergeRecordings(active, disk []models.RecordingStatusData) []models.RecordingStatusData {
	byKey := make(map[string]models.RecordingStatusData)
	key := func(r models.RecordingStatusData) string { return r.StreamID + "/" + r.RecordingID }
	for _, r := range disk {
		byKey[key(r)] = r
	}
	for _, r := range active {
		byKey[key(r)] = r // active wins (live segment count + Active flag)
	}
	out := make([]models.RecordingStatusData, 0, len(byKey))
	for _, r := range byKey {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

func mapRecordingError(err error) error {
	switch {
	case errors.Is(err, streaming.ErrAlreadyRecording):
		return huma.Error409Conflict(err.Error(), err)
	case errors.Is(err, streaming.ErrNotRecording):
		return huma.Error404NotFound(err.Error(), err)
	case errors.Is(err, streaming.ErrRecordingNotFound):
		return huma.Error404NotFound(err.Error(), err)
	case errors.Is(err, streaming.ErrRecordingActive):
		return huma.Error409Conflict(err.Error(), err)
	case errors.Is(err, streaming.ErrStreamNotFound):
		return huma.Error404NotFound("stream not found or pipeline disabled", err)
	default:
		return huma.Error500InternalServerError("internal server error", err)
	}
}
