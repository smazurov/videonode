//go:build planv2_tests

// Encoder upstream-resolution tests for the post-rewrite shape.
// buildEncoder picks the FrameSource based on parsed Upstream:
//   - "source:<id>" → ProducerFrameSource at SCMSocketPathFor(source.id)
//   - "composer:<id>" → ComposerFrameSource at SCMSocketPathFor(composer.id)
//   - dangling ref → error
//
// Awaits B1's real buildEncoder; here we exercise a local resolver
// that mirrors the post-B1 contract.
package pipeline

import (
	"fmt"
	"strings"
	"testing"
)

// resolveUpstream is the test-time analogue of post-B1 buildEncoder's
// Upstream → FrameSource resolution. Returns an opaque "socket" string
// the encoder will eventually dial; tests assert on the returned socket
// to pin the wiring.
func resolveUpstream(
	upstream string,
	sources map[string]PlanSource,
	composers map[string]PlanComposer,
) (socket string, kind string, err error) {
	k, id, ok := ParseUpstreamRef(upstream)
	if !ok {
		return "", "", fmt.Errorf("malformed upstream: %q", upstream)
	}
	switch k {
	case "source":
		if _, found := sources[id]; !found {
			return "", "", fmt.Errorf("upstream source %q not found", id)
		}
		return "/tmp/vn-bus-" + id + ".sock", "source", nil
	case "composer":
		if _, found := composers[id]; !found {
			return "", "", fmt.Errorf("upstream composer %q not found", id)
		}
		return "/tmp/vn-bus-composer-" + id + ".sock", "composer", nil
	default:
		return "", "", fmt.Errorf("upstream kind %q not source|composer", k)
	}
}

func TestEncoder_ResolvesSourceUpstream(t *testing.T) {
	sources := map[string]PlanSource{"hdmi0": {ID: "hdmi0", Device: "/dev/video0"}}
	composers := map[string]PlanComposer{}
	sock, kind, err := resolveUpstream("source:hdmi0", sources, composers)
	if err != nil {
		t.Fatalf("resolveUpstream: %v", err)
	}
	if kind != "source" {
		t.Errorf("kind = %q, want source", kind)
	}
	if !strings.Contains(sock, "vn-bus-hdmi0") {
		t.Errorf("socket %q missing producer scm-name", sock)
	}
}

func TestEncoder_ResolvesComposerUpstream(t *testing.T) {
	sources := map[string]PlanSource{"hdmi0": {ID: "hdmi0", Device: "/dev/video0"}}
	composers := map[string]PlanComposer{
		"main": {
			ID:     "main",
			Canvas: PlanCanvasDims{W: 1920, H: 1080},
			Inputs: []PlanComposerInput{{Ref: "source:hdmi0"}},
		},
	}
	sock, kind, err := resolveUpstream("composer:main", sources, composers)
	if err != nil {
		t.Fatalf("resolveUpstream: %v", err)
	}
	if kind != "composer" {
		t.Errorf("kind = %q, want composer", kind)
	}
	if !strings.Contains(sock, "vn-bus-composer-main") {
		t.Errorf("socket %q missing composer scm-name", sock)
	}
}

func TestEncoder_DanglingUpstreamRefIsError(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr string
	}{
		{"unknown source", "source:ghost", "source \"ghost\" not found"},
		{"unknown composer", "composer:ghost", "composer \"ghost\" not found"},
		{"malformed ref", "not-a-ref", "malformed upstream"},
		{"empty ref", "", "malformed upstream"},
		{"unsupported kind", "device:hdmi0", "kind \"device\" not source|composer"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := resolveUpstream(tc.ref, nil, nil)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// The four sub-tests below pin the existing EncoderStage shell-pipe
// contract — these survive the refactor unchanged because EncoderStage
// is plumbing-level and doesn't care about how its FrameSource was
// picked. They live in this file so the encoder package retains
// coverage of NV12/BGRA/custom-args/audio plumbing post-rewrite.

func TestEncoderStage_NV12_Y4M_BuildsYuv4mpegpipeInput(t *testing.T) {
	e := &EncoderStage{
		OwnerStreamID: "cam-front",
		Media: MediaSource{
			Video: ProducerFrameSource{Socket: "/tmp/vn-bus-cam.sock"},
		},
		Cfg:       EncoderConfig{Codec: "h264", EncoderName: "h264_rkmpp", Bitrate: "4M", GOP: 60},
		Publish:   []PublishTarget{{Type: "rtsp", URL: "rtsp://localhost:8554/cam-front"}},
		VNSinkBin: "/usr/local/bin/vn-sink",
	}
	argv, _, err := e.Command()
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	cmd := argv[2]
	for _, want := range []string{
		"vn-sink --socket /tmp/vn-bus-cam.sock",
		"-f yuv4mpegpipe -i pipe:0",
		"-c:v h264_rkmpp",
		"-f rtsp rtsp://localhost:8554/cam-front",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in: %s", want, cmd)
		}
	}
}

func TestEncoderStage_BGRA_RawBuildsRawvideoInput(t *testing.T) {
	e := &EncoderStage{
		OwnerStreamID: "canvas-1",
		Media: MediaSource{
			Video: ComposerFrameSource{
				Socket: "/tmp/vn-bus-composer-canvas-1.sock",
				Width:  3840, Height: 1080, Fps: 30,
			},
		},
		Cfg:       EncoderConfig{Codec: "h265", EncoderName: "hevc_rkmpp", Bitrate: "12M"},
		Publish:   []PublishTarget{{Type: "rtsp", URL: "rtsp://localhost:8554/canvas-1"}},
		VNSinkBin: "/usr/local/bin/vn-sink",
	}
	argv, _, err := e.Command()
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	cmd := argv[2]
	for _, want := range []string{
		"-f rawvideo",
		"-pix_fmt bgra",
		"-s 3840x1080",
		"-framerate 30",
		"-c:v hevc_rkmpp",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in: %s", want, cmd)
		}
	}
}

func TestEncoderStage_CustomEncoderArgsReplacesTail(t *testing.T) {
	e := &EncoderStage{
		OwnerStreamID: "cam",
		Media: MediaSource{
			Video: ProducerFrameSource{Socket: "/tmp/sock"},
		},
		CustomEncoderArgs: "-c:v libx264 -preset ultrafast -f rtsp rtsp://localhost:8554/cam",
		VNSinkBin:         "/usr/bin/vn-sink",
	}
	argv, _, err := e.Command()
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	cmd := argv[2]
	if !strings.Contains(cmd, "-c:v libx264") {
		t.Errorf("custom encoder not honored: %s", cmd)
	}
	if strings.Contains(cmd, "h264_rkmpp") {
		t.Errorf("default encoder leaked through: %s", cmd)
	}
}

func TestEncoderStage_AudioInputsAppendedPerDevice(t *testing.T) {
	e := &EncoderStage{
		OwnerStreamID: "cam",
		Media: MediaSource{
			Video: ProducerFrameSource{Socket: "/tmp/sock"},
			Audio: ALSADirectAudio{Config: AudioConfig{Devices: []string{"hw:0", "hw:1"}}},
		},
		Cfg:       EncoderConfig{Codec: "h264", Bitrate: "2M"},
		Publish:   []PublishTarget{{Type: "rtsp", URL: "rtsp://x/y"}},
		VNSinkBin: "/usr/bin/vn-sink",
	}
	argv, _, err := e.Command()
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	cmd := argv[2]
	for _, want := range []string{"-f alsa", "-i hw:0", "-i hw:1"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in: %s", want, cmd)
		}
	}
}
