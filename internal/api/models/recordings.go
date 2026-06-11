package models

import "time"

// RecordingStatusData is the API view of a recording session.
type RecordingStatusData struct {
	RecordingID      string    `json:"recording_id" example:"20260610T120000Z" doc:"Recording session id"`
	StreamID         string    `json:"stream_id" example:"stream-001" doc:"Recorded stream"`
	Active           bool      `json:"active" example:"true" doc:"Whether the recording is currently capturing"`
	StartedAt        time.Time `json:"started_at" doc:"UTC start time"`
	Segments         int       `json:"segments" example:"12" doc:"HLS segments written so far"`
	SizeBytes        int64     `json:"size_bytes" example:"12582912" doc:"Bytes on disk for this session"`
	DurationSeconds  float64   `json:"duration_seconds" example:"95.7" doc:"Recorded length in seconds (elapsed time while active)"`
	PlaylistURL      string    `json:"playlist_url,omitempty" example:"/api/streams/stream-001/recordings/20260610T120000Z/index.m3u8" doc:"HLS playlist URL"`
	ThumbnailsVTTURL string    `json:"thumbnails_vtt_url,omitempty" doc:"WebVTT storyboard track URL (Media Chrome hover preview)"`
}

// RecordingResponse wraps a single recording status.
type RecordingResponse struct {
	Body RecordingStatusData
}

// RecordingListData is the list-recordings payload.
type RecordingListData struct {
	Recordings []RecordingStatusData `json:"recordings" doc:"Recording sessions (active + on-disk)"`
	Count      int                   `json:"count" example:"3" doc:"Number of recordings"`
}

// RecordingListResponse wraps the recording list.
type RecordingListResponse struct {
	Body RecordingListData
}
