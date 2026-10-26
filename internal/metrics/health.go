package metrics

import "time"

// HealthTimeout is the inactivity window for media and encoder progress.
const HealthTimeout = 5 * time.Second

// ActivityHealth describes delivery freshness, not the cause of missing data.
type ActivityHealth struct {
	State string `json:"state" enum:"never_received,flowing,stalled,unsupported"`
	Count uint64 `json:"count"`
	AgeMS *int64 `json:"age_ms,omitempty" doc:"Milliseconds since the last observation; absent before the first observation"`
}

// Activity records arrivals using monotonic time. Its owner provides synchronization.
type Activity struct {
	Count uint64
	Last  time.Time
}

// Record notes one arrival. Payload content (including silence) is irrelevant.
func (a *Activity) Record(now time.Time) {
	a.Count++
	a.Last = now
}

// Snapshot evaluates freshness without needing a background timer.
func (a Activity) Snapshot(now time.Time) ActivityHealth {
	out := ActivityHealth{State: "never_received", Count: a.Count}
	if a.Last.IsZero() {
		return out
	}
	age := max(now.Sub(a.Last), 0)
	ms := age.Milliseconds()
	out.AgeMS = &ms
	out.State = "flowing"
	if age >= HealthTimeout {
		out.State = "stalled"
	}
	return out
}

// FFmpegHealth combines the last reported values with independent freshness signals.
type FFmpegHealth struct {
	Progress ActivityHealth      `json:"progress"`
	Frames   ActivityHealth      `json:"frames" doc:"Frame count and time since it last advanced, independent of progress-report arrivals"`
	Stats    FFmpegStreamMetrics `json:"stats"`
}

// EncoderHealth identifies the process run owning the progress observations.
type EncoderHealth struct {
	State       string       `json:"state" enum:"idle,starting,running,stopping,error"`
	StartedAtUS int64        `json:"started_at_us" doc:"Encoder run identity; zero when no run is registered"`
	FFmpeg      FFmpegHealth `json:"ffmpeg"`
}
