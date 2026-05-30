package streaming

// StreamStatusPayload is the payload of a "stream.status" SSE event: the
// encoder's collapsed run state, flipped on RTSP-producer connect and
// last-reader-gone.
type StreamStatusPayload struct {
	State     string `json:"state"`
	EncoderUp bool   `json:"encoder_up"`
}

// StreamMetricsPayload is the payload of a "stream.metrics" SSE event.
type StreamMetricsPayload struct {
	FPS             float64 `json:"fps"`
	DroppedFrames   float64 `json:"dropped_frames"`
	DuplicateFrames float64 `json:"duplicate_frames"`
	BytesOut        float64 `json:"bytes_out"`
	PacketsOut      float64 `json:"packets_out"`
}

// StreamConsumersPayload is the payload of a "stream.consumers" SSE event:
// per-protocol reader counts plus optional per-client telemetry.
type StreamConsumersPayload struct {
	Total         int                `json:"total"`
	RTSP          int                `json:"rtsp"`
	WebRTC        int                `json:"webrtc"`
	SRT           int                `json:"srt"`
	WebRTCClients []WebRTCClientInfo `json:"webrtc_clients,omitempty"`
	SRTClients    []SRTClientInfo    `json:"srt_clients,omitempty"`
}
