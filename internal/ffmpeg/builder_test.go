package ffmpeg

import (
	"os/exec"
	"slices"
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

func TestBuildCommand_PipeInput_ColorTags(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe: &PipeInput{
			Format:      "rawvideo",
			PixelFormat: "nv12",
			Width:       1920,
			Height:      1080,
			FPS:         30,
			Color:       ColorTags{Space: "bt709", Primaries: "bt709", TRC: "bt709", Range: "tv"},
		},
		Encoder: "h264_rkmpp",
		Outputs: []OutputTarget{{Type: "rtsp", URL: "rtsp://x"}},
	})

	// Input side: color flags before -i pipe:0.
	if !strings.Contains(got, "-colorspace bt709 -color_primaries bt709 -color_trc bt709 -color_range tv -i pipe:0") {
		t.Errorf("expected input-side VUI flags before -i; got %q", got)
	}
	// Output side: color flags after -c:v.
	if !strings.Contains(got, "-c:v h264_rkmpp -colorspace bt709 -color_primaries bt709 -color_trc bt709 -color_range tv") {
		t.Errorf("expected output-side VUI flags after -c:v; got %q", got)
	}
}

func TestBuildCommand_NutPipeInput(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe: &PipeInput{
			Format: "nut",
			Color:  ColorTags{Space: "bt709", Range: "tv"},
		},
		Encoder: "h264_rkmpp",
		Outputs: []OutputTarget{{Type: "rtsp", URL: "rtsp://x"}},
	})

	if !strings.Contains(got, "-probesize 32 -analyzeduration 0 -f nut -colorspace bt709 -color_range tv -i pipe:0") {
		t.Errorf("expected nut pipe input with input-side color tags; got %q", got)
	}
	for _, unwanted := range []string{"-pix_fmt", "-framerate", "-filter_complex", "-c:a", "-f alsa"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("video-only nut input must not emit %q; got %q", unwanted, got)
		}
	}
}

func TestBuildCommand_NutPipeAudioTracks(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe: &PipeInput{Format: "nut", AudioTracks: 2},
		Encoder:   "libx264",
		Outputs:   []OutputTarget{{Type: "rtsp", URL: "rtsp://x"}},
	})

	for _, want := range []string{
		"-filter_complex '[0:a:0]aresample=async=1000:min_hard_comp=0.100000:first_pts=0[a0];[0:a:1]aresample=async=1000:min_hard_comp=0.100000:first_pts=0[a1]'",
		"-map 0:v:0 -map '[a0]' -map '[a1]'",
		"-c:a libopus -b:a 128k -ar 48000",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "-f alsa") || strings.Contains(got, "-thread_queue_size") {
		t.Errorf("nut pipe audio must not open ALSA in ffmpeg; got %q", got)
	}
}

func TestBuildCommand_NutPipeAudio_CodecBitrateAndMix(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe:    &PipeInput{Format: "nut", AudioTracks: 2},
		Encoder:      "libx264",
		AudioCodec:   "aac",
		AudioBitrate: "192k",
		AudioFilters: "amix=inputs=2:duration=shortest",
		Outputs:      []OutputTarget{{Type: "rtsp", URL: "rtsp://x"}},
	})

	for _, want := range []string{
		"-filter_complex '[0:a:0]aresample=async=1000:min_hard_comp=0.100000:first_pts=0[s0];[0:a:1]aresample=async=1000:min_hard_comp=0.100000:first_pts=0[s1];[s0][s1]amix=inputs=2:duration=shortest[aout]'",
		"-map 0:v:0 -map '[aout]'",
		"-c:a aac -b:a 192k -ar 48000",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, " -af ") {
		t.Errorf("pipe audio must not emit -af, got %q", got)
	}
}

func TestBuildCommand_AudioTracksIgnoredForNonNutPipe(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe: &PipeInput{Format: "yuv4mpegpipe", AudioTracks: 2},
		Encoder:   "libx264",
		Outputs:   []OutputTarget{{Type: "rtsp", URL: "rtsp://x"}},
	})
	if strings.Contains(got, "-filter_complex") || strings.Contains(got, "-c:a") {
		t.Errorf("audio tracks only exist inside nut; got %q", got)
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
		"-thread_queue_size 8192",
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

func TestBuildInputArgs_NutPipeWithAudio(t *testing.T) {
	got := BuildInputArgs(&Params{
		InputPipe: &PipeInput{Format: "nut", AudioTracks: 2},
	})
	if !strings.Contains(got, "-f nut -i pipe:0") {
		t.Errorf("expected nut pipe input; got %q", got)
	}
	for _, unwanted := range []string{"-filter_complex", "-f alsa", "-c:v"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("BuildInputArgs must not emit %q; got %q", unwanted, got)
		}
	}
}

// TestBuildCommand_NutPipeAudioSurvivesShell runs the command through sh and
// checks the filter graph and map labels arrive as single, unglobbed args.
func TestBuildCommand_NutPipeAudioSurvivesShell(t *testing.T) {
	got := BuildCommand(&Params{
		InputPipe:    &PipeInput{Format: "nut", AudioTracks: 2},
		Encoder:      "libx264",
		AudioFilters: "amix=inputs=2:weights='1 0.5'",
		Outputs:      []OutputTarget{{Type: "rtsp", URL: "rtsp://x"}},
	})
	dir := t.TempDir()
	for _, glob := range []string{"a", "0"} {
		if err := exec.Command("touch", dir+"/"+glob).Run(); err != nil {
			t.Fatal(err)
		}
	}
	script := "printf '%s\\n' " + strings.TrimPrefix(got, "ffmpeg ")
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sh rejected the command: %v\n%s", err, got)
	}
	args := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := "[0:a:0]aresample=async=1000:min_hard_comp=0.100000:first_pts=0[s0];" +
		"[0:a:1]aresample=async=1000:min_hard_comp=0.100000:first_pts=0[s1];" +
		"[s0][s1]amix=inputs=2:weights='1 0.5'[aout]"
	if !slices.Contains(args, want) {
		t.Errorf("filter graph not delivered as one argument; got args %q", args)
	}
	if !slices.Contains(args, "[aout]") {
		t.Errorf("map label not delivered intact; got args %q", args)
	}
}
