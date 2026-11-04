package pipeline

import (
	"strings"
	"testing"
)

func newTestEncoder(audio AudioConfig, fps int) *EncoderStage {
	return &EncoderStage{
		OwnerStreamID: "s1",
		Media: MediaSource{
			Video: ComposerFrameSource{Socket: "/tmp/vn-bus-composer-c.sock", Width: 1920, Height: 1080, Fps: fps},
			Audio: SinkAudio{Config: audio},
		},
		Resolved:  EncoderResolution{EncoderName: "libx264"},
		OutputURL: "rtsp://localhost:8554/s1",
		VNSinkBin: "/usr/bin/videonode-sink",
	}
}

func encoderShell(t *testing.T, e *EncoderStage) (string, string) {
	t.Helper()
	argv, _, err := e.Command()
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if len(argv) != 3 || argv[0] != "/bin/sh" {
		t.Fatalf("expected sh -c argv, got %q", argv)
	}
	sinkPart, ffmpegPart, ok := strings.Cut(argv[2], " | ")
	if !ok {
		t.Fatalf("expected vn-sink | ffmpeg pipeline, got %q", argv[2])
	}
	return sinkPart, ffmpegPart
}

func TestEncoderStage_Command_AudioCapturedByVNSink(t *testing.T) {
	t.Setenv("ALSA_CONFIG_PATH", "/usr/share/videonode/videonode.asound.conf")
	e := newTestEncoder(AudioConfig{Devices: []string{"hw:Lyra,0", "hw:3,0"}, Codec: "opus", OffsetMS: 80}, 30)
	sink, ff := encoderShell(t, e)

	for _, want := range []string{
		"/usr/bin/videonode-sink --socket /tmp/vn-bus-composer-c.sock --mux nut --fps 30",
		"--audio-device vncap:CARD=Lyra,DEV=0 --audio-device vncap:CARD=3,DEV=0",
		"--audio-rate 48000 --audio-channels 2 --audio-offset-ms 80",
	} {
		if !strings.Contains(sink, want) {
			t.Errorf("vn-sink argv missing %q in %q", want, sink)
		}
	}
	for _, want := range []string{"-f nut", "-i pipe:0", "[0:a:0]", "[0:a:1]", "-map 0:v:0 -map '[a0]' -map '[a1]'", "-c:a libopus"} {
		if !strings.Contains(ff, want) {
			t.Errorf("ffmpeg missing %q in %q", want, ff)
		}
	}
	if strings.Contains(ff, "-f alsa") {
		t.Errorf("ffmpeg must not open ALSA; got %q", ff)
	}
}

func TestEncoderStage_Command_VideoOnly(t *testing.T) {
	sink, ff := encoderShell(t, newTestEncoder(AudioConfig{}, 60))
	if !strings.HasSuffix(sink, "--mux nut --fps 60") {
		t.Errorf("unexpected vn-sink argv %q", sink)
	}
	if strings.Contains(ff, "-c:a") || strings.Contains(ff, "-filter_complex") {
		t.Errorf("video-only stream must not configure audio; got %q", ff)
	}
}

func TestEncoderStage_Command_DefaultFPSAndOffsetClamp(t *testing.T) {
	sink, _ := encoderShell(t, newTestEncoder(AudioConfig{Devices: []string{"plughw:1,0"}, OffsetMS: -9000}, 0))
	for _, want := range []string{"--fps 30", "--audio-device plughw:1,0", "--audio-offset-ms -2000"} {
		if !strings.Contains(sink, want) {
			t.Errorf("vn-sink argv missing %q in %q", want, sink)
		}
	}
}

func TestEncoderStage_Command_ZeroOffsetOmitted(t *testing.T) {
	sink, _ := encoderShell(t, newTestEncoder(AudioConfig{Devices: []string{"hw:0,0"}}, 30))
	if strings.Contains(sink, "--audio-offset-ms") {
		t.Errorf("zero offset should be omitted; got %q", sink)
	}
}
