package streaming

// WebRTCClientInfo is the per-peer payload inside the stream.consumers SSE event.
type WebRTCClientInfo struct {
	ID             string  `json:"id"`
	ClientIP       string  `json:"client_ip"`
	ConnectedSince string  `json:"connected_since"`
	BytesSent      int64   `json:"bytes_sent"`
	JitterMs       float64 `json:"jitter_ms"`
}

// SRTClientInfo is the per-consumer payload inside the stream.consumers SSE event.
type SRTClientInfo struct {
	ID             string  `json:"id"`
	ClientIP       string  `json:"client_ip"`
	ConnectedSince string  `json:"connected_since"`
	BytesSent      int64   `json:"bytes_sent"`
	RTTMs          float64 `json:"rtt_ms"`
}

// RTSPClientInfo is the per-reader payload inside the stream.consumers SSE event.
type RTSPClientInfo struct {
	ID             string `json:"id"`
	ClientIP       string `json:"client_ip"`
	ConnectedSince string `json:"connected_since"`
}
