package collectors

import (
	"net"
	"path/filepath"
	"testing"
	"time"
)

const lyraAbsentBlock = "video.upstream=connected\n" +
	"video.repeated_frames=12\n" +
	"video.dropped_frames=0\n" +
	"audio.count=1\n" +
	"audio.0.device=vncap:CARD=Lyra,DEV=0\n" +
	"audio.0.state=absent\n" +
	"audio.0.state_age_ms=9700\n" +
	"audio.0.error=vncap:CARD=Lyra,DEV=0: No such device\n" +
	"audio.0.xruns=1\n" +
	"audio.0.silence_ms=9400\n" +
	"audio.0.dropped_ms=0\n" +
	"status=continue\n"

func TestParseSinkStatus(t *testing.T) {
	now := time.UnixMicro(1_000_000_000_000)
	tests := []struct {
		name         string
		data         map[string]string
		wantUpstream string
		wantAudio    int
		check        func(t *testing.T, state string, sinceUS int64, rate float64)
	}{
		{
			name: "absent device keeps age as absolute since",
			data: map[string]string{
				"video.upstream": "connected", "audio.count": "1",
				"audio.0.device": "hw:Lyra,0", "audio.0.state": "absent", "audio.0.state_age_ms": "2500",
				"audio.0.rate_hz": "48000.94",
			},
			wantUpstream: "connected",
			wantAudio:    1,
			check: func(t *testing.T, state string, sinceUS int64, rate float64) {
				if state != "absent" || sinceUS != now.UnixMicro()-2_500_000 || rate != 48000.94 {
					t.Errorf("state=%s since=%d rate=%v", state, sinceUS, rate)
				}
			},
		},
		{
			name:         "unknown values fall back safely",
			data:         map[string]string{"video.upstream": "weird", "audio.count": "1", "audio.0.state": "exploded"},
			wantUpstream: "unknown",
			wantAudio:    1,
			check: func(t *testing.T, state string, sinceUS int64, _ float64) {
				if state != "opening" || sinceUS != 0 {
					t.Errorf("state=%s since=%d", state, sinceUS)
				}
			},
		},
		{
			name:         "audio count is capped",
			data:         map[string]string{"video.upstream": "reconnecting", "audio.count": "999"},
			wantUpstream: "reconnecting",
			wantAudio:    maxSinkAudio,
		},
		{
			name:         "missing count means no audio",
			data:         map[string]string{},
			wantUpstream: "unknown",
			wantAudio:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			video, audio := parseSinkStatus(tt.data, now)
			if video.Upstream != tt.wantUpstream {
				t.Errorf("upstream = %q, want %q", video.Upstream, tt.wantUpstream)
			}
			if len(audio) != tt.wantAudio {
				t.Fatalf("audio entries = %d, want %d", len(audio), tt.wantAudio)
			}
			if tt.check != nil {
				tt.check(t, audio[0].State, audio[0].SinceUS, audio[0].RateHz)
			}
		})
	}
}

func startCollector(t *testing.T) *EncoderCollector {
	t.Helper()
	skipOnMacOS(t)
	c := NewEncoderCollector(filepath.Join(t.TempDir(), "progress.sock"), t.Name())
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop() })
	return c
}

func dialAndWrite(t *testing.T, c *EncoderCollector, payload string) net.Conn {
	t.Helper()
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	return conn
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEncoderCollector_SinkHelloRoutesToSinkHealth(t *testing.T) {
	c := startCollector(t)
	dialAndWrite(t, c, "sink_wire=1\n"+lyraAbsentBlock)
	waitUntil(t, "sink report", func() bool { return c.SinkHealth(0, time.Now()).Reports.Count == 1 })

	sink := c.SinkHealth(0, time.Now())
	if sink.Reports.State != "flowing" || sink.Video.Upstream != "connected" || sink.Video.RepeatedFrames != 12 {
		t.Fatalf("sink health = %+v", sink)
	}
	if len(sink.Audio) != 1 || sink.Audio[0].State != "absent" || sink.Audio[0].SilenceMS != 9400 {
		t.Fatalf("audio = %+v", sink.Audio)
	}
	if ff := c.Health(0, time.Now()); ff.Progress.State != "never_received" {
		t.Fatalf("sink report leaked into ffmpeg progress: %+v", ff)
	}
}

func TestEncoderCollector_FFmpegAndSinkShareTheSocket(t *testing.T) {
	c := startCollector(t)
	dialAndWrite(t, c, "frame=30\nfps=30\nspeed=1x\nprogress=continue\n")
	dialAndWrite(t, c, "sink_wire=1\n"+lyraAbsentBlock)
	waitUntil(t, "both reports", func() bool {
		return c.Health(0, time.Now()).Progress.Count == 1 && c.SinkHealth(0, time.Now()).Reports.Count == 1
	})
	if ff := c.Health(0, time.Now()); ff.Frames.Count != 30 || ff.Stats.Speed != 1 {
		t.Fatalf("ffmpeg health = %+v", ff)
	}
}

func TestEncoderCollector_UnsupportedSinkWireIsRejected(t *testing.T) {
	c := startCollector(t)
	conn := dialAndWrite(t, c, "sink_wire=2\n"+lyraAbsentBlock)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	var buf [1]byte
	if _, err := conn.Read(buf[:]); err == nil {
		t.Fatal("collector kept an unsupported sink connection open")
	}
	if c.SinkHealth(0, time.Now()).Reports.Count != 0 || c.Health(0, time.Now()).Progress.Count != 0 {
		t.Fatal("unsupported wire version produced health")
	}
}

func TestEncoderCollector_BeginRunClearsSinkHealth(t *testing.T) {
	c := startCollector(t)
	dialAndWrite(t, c, "sink_wire=1\n"+lyraAbsentBlock)
	waitUntil(t, "sink report", func() bool { return c.SinkHealth(0, time.Now()).Reports.Count == 1 })
	if err := c.BeginRun(42); err != nil {
		t.Fatal(err)
	}
	sink := c.SinkHealth(42, time.Now())
	if sink.Reports.State != "never_received" || len(sink.Audio) != 0 || sink.Video.Upstream != "unknown" {
		t.Fatalf("previous run leaked into reset sink health: %+v", sink)
	}
	if c.SinkHealth(0, time.Now()).Reports.Count != 0 {
		t.Fatal("stale run id returned observations")
	}
}

func TestEncoderCollector_SinkBlocksWithoutHelloAreFFmpeg(t *testing.T) {
	c := startCollector(t)
	dialAndWrite(t, c, lyraAbsentBlock+"progress=continue\n")
	waitUntil(t, "progress", func() bool { return c.Health(0, time.Now()).Progress.Count == 1 })
	if c.SinkHealth(0, time.Now()).Reports.Count != 0 {
		t.Fatal("sink keys without a hello were treated as a sink report")
	}
}
