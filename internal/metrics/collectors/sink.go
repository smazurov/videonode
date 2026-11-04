package collectors

import (
	"slices"
	"strconv"
	"time"

	"github.com/smazurov/videonode/internal/metrics"
)

// vn-sink opens every status connection with "sink_wire=<version>".
const (
	sinkHelloKey    = "sink_wire"
	sinkWireVersion = "1"
	maxSinkAudio    = 16
)

type sinkState struct {
	reports metrics.Activity
	video   metrics.SinkVideoHealth
	audio   []metrics.AudioInputHealth
}

// SinkHealth returns vn-sink's latest report for the given encoder run.
func (f *EncoderCollector) SinkHealth(runID int64, now time.Time) metrics.SinkHealth {
	f.mu.Lock()
	defer f.mu.Unlock()
	if runID != f.runID || f.sink.reports.Count == 0 {
		return metrics.EmptySinkHealth(now)
	}
	return metrics.SinkHealth{
		Reports: f.sink.reports.Snapshot(now),
		Video:   f.sink.video,
		Audio:   slices.Clone(f.sink.audio),
	}
}

func (f *EncoderCollector) applySinkStatus(data map[string]string, generation uint64, now time.Time) {
	video, audio := parseSinkStatus(data, now)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped || generation != f.generation {
		return
	}
	f.sink.reports.Record(now)
	f.sink.video = video
	f.sink.audio = audio
	metrics.SetSinkMetrics(f.streamID, video, audio)
}

// parseSinkStatus decodes one vn-sink status block. State ages become
// absolute times on receipt, so vn-sink never needs the wall clock.
func parseSinkStatus(data map[string]string, now time.Time) (metrics.SinkVideoHealth, []metrics.AudioInputHealth) {
	video := metrics.SinkVideoHealth{
		Upstream:       "unknown",
		RepeatedFrames: parseUint(data["video.repeated_frames"]),
		DroppedFrames:  parseUint(data["video.dropped_frames"]),
	}
	if up := data["video.upstream"]; up == "connected" || up == "reconnecting" {
		video.Upstream = up
	}
	count := min(int(parseUint(data["audio.count"])), maxSinkAudio)
	audio := make([]metrics.AudioInputHealth, 0, count)
	for k := range count {
		p := "audio." + strconv.Itoa(k) + "."
		in := metrics.AudioInputHealth{
			Track:     k,
			Device:    data[p+"device"],
			State:     data[p+"state"],
			Error:     data[p+"error"],
			Xruns:     parseUint(data[p+"xruns"]),
			SilenceMS: parseUint(data[p+"silence_ms"]),
			DroppedMS: parseUint(data[p+"dropped_ms"]),
		}
		switch in.State {
		case "opening", "live", "stalled", "absent":
		default:
			in.State = "opening"
		}
		if age, err := strconv.ParseInt(data[p+"state_age_ms"], 10, 64); err == nil && age >= 0 {
			in.SinceUS = now.Add(-time.Duration(age) * time.Millisecond).UnixMicro()
		}
		if rate, ok := parseProgressNumber(data[p+"rate_hz"]); ok && rate > 0 {
			in.RateHz = rate
		}
		audio = append(audio, in)
	}
	return video, audio
}

func parseUint(raw string) uint64 {
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
