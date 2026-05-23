package ffmpeg

import (
	"strings"
	"testing"
)

func TestBuildCommand_PipeInput_Y4M(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe: &PipeInput{Format: "yuv4mpegpipe"},
		Encoder:   "libx264",
		Bitrate:   "4M",
		Outputs:   []OutputTarget{{Type: "rtsp", URL: "rtsp://127.0.0.1:8554/s"}},
	})

	if strings.Contains(got, "-nostdin") {
		t.Errorf("expected -nostdin to be stripped for pipe input; got %q", got)
	}
	if !strings.Contains(got, "-f yuv4mpegpipe -i pipe:0") {
		t.Errorf("expected Y4M pipe input; got %q", got)
	}
	if !strings.Contains(got, "-c:v libx264") {
		t.Errorf("expected libx264 encoder; got %q", got)
	}
	if !strings.Contains(got, "-rtsp_transport tcp -f rtsp rtsp://127.0.0.1:8554/s") {
		t.Errorf("expected rtsp output; got %q", got)
	}
}

func TestBuildCommand_PipeInput_BGRA(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe: &PipeInput{
			Format:      "rawvideo",
			PixelFormat: "bgra",
			Width:       1920,
			Height:      1080,
			FPS:         30,
		},
		Encoder: "h264_rkmpp",
		Outputs: []OutputTarget{{Type: "rtsp", URL: "rtsp://x"}},
	})

	if !strings.Contains(got, "-f rawvideo -pix_fmt bgra -s 1920x1080 -framerate 30 -i pipe:0") {
		t.Errorf("expected rawvideo BGRA pipe input; got %q", got)
	}
	if strings.Contains(got, "-nostdin") {
		t.Errorf("expected -nostdin stripped; got %q", got)
	}
}

func TestBuildCommand_MultiAudio_FilterComplex(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe:   &PipeInput{Format: "yuv4mpegpipe"},
		Encoder:     "libx264",
		AudioInputs: []string{"hw:0,0", "hw:1,0"},
		Outputs:     []OutputTarget{{Type: "rtsp", URL: "rtsp://x"}},
	})

	for _, want := range []string{
		"-thread_queue_size 1024 -f alsa -sample_fmt s16 -ar 48000 -ac 2 -i hw:0,0",
		"-thread_queue_size 1024 -f alsa -sample_fmt s16 -ar 48000 -ac 2 -i hw:1,0",
		"-filter_complex [1:a]aresample=async=1:min_hard_comp=0.100000:first_pts=0[a0];[2:a]aresample=async=1:min_hard_comp=0.100000:first_pts=0[a1]",
		"-map 0:v -map [a0] -map [a1]",
		"-c:a libopus -b:a 128k -ar 48000",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestBuildCommand_MultiOutput(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe: &PipeInput{Format: "yuv4mpegpipe"},
		Encoder:   "libx264",
		Outputs: []OutputTarget{
			{Type: "rtsp", URL: "rtsp://a"},
			{Type: "srt", URL: "srt://b"},
		},
	})

	if !strings.Contains(got, "-rtsp_transport tcp -f rtsp rtsp://a") {
		t.Errorf("missing rtsp output; got %q", got)
	}
	if !strings.Contains(got, "-muxdelay 0 -muxpreload 0 -flush_packets 1 -f mpegts srt://b") {
		t.Errorf("missing srt output; got %q", got)
	}
	// -rtsp_transport tcp should appear exactly once
	if c := strings.Count(got, "-rtsp_transport tcp"); c != 1 {
		t.Errorf("expected -rtsp_transport tcp exactly once; got %d in %q", c, got)
	}
}

func TestBuildCommand_BackCompat_V4L2_Unchanged(t *testing.T) {
	// Regression: a v4l2-style call with single AudioDevice + single
	// OutputURL must keep producing the legacy command shape. If this
	// drifts, cmd/stream.go consumers break silently.
	got := BuildCommand(&Params{
		DevicePath:  "/dev/video0",
		InputFormat: "nv12",
		Resolution:  "1920x1080",
		FPS:         "30",
		Encoder:     "h264_rkmpp",
		Bitrate:     "4M",
		AudioDevice: "hw:0,0",
		BFrames:     0,
		OutputURL:   "rtsp://127.0.0.1:8554/s",
	})

	for _, want := range []string{
		"-nostdin",
		"-f v4l2",
		"-input_format nv12",
		"-video_size 1920x1080",
		"-framerate 30",
		"-i /dev/video0",
		"-thread_queue_size 1024",
		"-f alsa -sample_fmt s16 -ar 48000 -ac 2",
		"-i hw:0,0",
		"-map 0:v -map 1:a",
		"-c:v h264_rkmpp",
		"-rtsp_transport tcp -f rtsp rtsp://127.0.0.1:8554/s",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestBuildInputArgs_PipeY4M(t *testing.T) {
	got := BuildInputArgs(&Params{
		InputPipe: &PipeInput{Format: "yuv4mpegpipe"},
	})
	if !strings.Contains(got, "-f yuv4mpegpipe -i pipe:0") {
		t.Errorf("expected Y4M pipe input; got %q", got)
	}
	if strings.Contains(got, "-c:v") {
		t.Errorf("BuildInputArgs must not emit encoder args; got %q", got)
	}
}

func TestBuildInputArgs_PipeWithMultiAudio(t *testing.T) {
	got := BuildInputArgs(&Params{
		InputPipe:   &PipeInput{Format: "yuv4mpegpipe"},
		AudioInputs: []string{"hw:0,0", "hw:1,0"},
	})
	if !strings.Contains(got, "-i hw:0,0") || !strings.Contains(got, "-i hw:1,0") {
		t.Errorf("missing audio inputs; got %q", got)
	}
	if strings.Contains(got, "-filter_complex") {
		t.Errorf("BuildInputArgs must not emit -filter_complex; got %q", got)
	}
}
