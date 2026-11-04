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
	Sink        SinkHealth   `json:"sink" required:"false" doc:"vn-sink input status; absent from daemons that predate sink reports"`
}

// SinkHealth is vn-sink's view of the encoder's inputs: the video upstream
// and every audio device it captures. Only vn-sink can tell real audio from
// the silence it substitutes, so a flowing audio track alone proves nothing.
type SinkHealth struct {
	Reports ActivityHealth     `json:"reports" doc:"Freshness of vn-sink status reports for this run"`
	Video   SinkVideoHealth    `json:"video"`
	Audio   []AudioInputHealth `json:"audio" doc:"One entry per configured audio device, in output track order"`
}

// SinkVideoHealth reports vn-sink's connection to the upstream source or composer.
type SinkVideoHealth struct {
	Upstream       string `json:"upstream" enum:"unknown,connected,reconnecting" doc:"Producer socket state as seen by vn-sink"`
	RepeatedFrames uint64 `json:"repeated_frames" doc:"Frames repeated to cover upstream gaps this run"`
	DroppedFrames  uint64 `json:"dropped_frames" doc:"Frames dropped for arriving faster than the stream rate this run"`
}

// AudioInputHealth is one ALSA device as vn-sink captures it.
type AudioInputHealth struct {
	Track     int     `json:"track" doc:"Audio track index in the published stream"`
	Device    string  `json:"device" doc:"ALSA capture device vn-sink opened"`
	State     string  `json:"state" enum:"opening,live,stalled,absent" doc:"live delivers samples; stalled and absent are covered with silence"`
	SinceUS   int64   `json:"since_us,omitempty" doc:"Unix microseconds when the device entered its current state"`
	Error     string  `json:"error,omitempty" doc:"Why the device is absent, e.g. No such device"`
	Xruns     uint64  `json:"xruns" doc:"Capture overruns this run"`
	SilenceMS uint64  `json:"silence_ms" doc:"Silence substituted for missing device audio this run"`
	DroppedMS uint64  `json:"dropped_ms" doc:"Device audio dropped for arriving too late this run"`
	RateHz    float64 `json:"rate_hz,omitempty" doc:"Estimated device sample rate"`
}

// EmptySinkHealth is the sink section before any report for the current run.
func EmptySinkHealth(now time.Time) SinkHealth {
	return SinkHealth{
		Reports: Activity{}.Snapshot(now),
		Video:   SinkVideoHealth{Upstream: "unknown"},
		Audio:   []AudioInputHealth{},
	}
}
