package pipeline

import (
	"strings"
	"testing"
)

func TestEncoder_NV12_Y4M_BuildsYuv4mpegpipeInput(t *testing.T) {
	e := &EncoderStage{
		StreamID_: "cam-front",
		Media: MediaSource{
			Video: ProducerFrameSource{Socket: "/tmp/vn-bus-cam.sock"},
		},
		Cfg:       EncoderConfig{Codec: "h264", Bitrate: "4M", GOP: 60},
		Publish:   []PublishTarget{{Type: "rtsp", URL: "rtsp://localhost:8554/cam-front"}},
		VNSinkBin: "/usr/local/bin/vn-sink",
	}
	argv, _, err := e.Command()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	if len(argv) != 3 || argv[0] != "/bin/sh" || argv[1] != "-c" {
		t.Fatalf("expected /bin/sh -c <cmd>, got %v", argv)
	}
	cmd := argv[2]
	if !strings.Contains(cmd, "vn-sink --socket /tmp/vn-bus-cam.sock") {
		t.Errorf("vn-sink fragment missing in: %s", cmd)
	}
	if !strings.Contains(cmd, "-f yuv4mpegpipe -i pipe:0") {
		t.Errorf("NV12 input fragment missing in: %s", cmd)
	}
	if !strings.Contains(cmd, "-c:v h264_rkmpp") {
		t.Errorf("expected h264_rkmpp encoder in: %s", cmd)
	}
	if !strings.Contains(cmd, "-f rtsp rtsp://localhost:8554/cam-front") {
		t.Errorf("RTSP publish missing in: %s", cmd)
	}
}

func TestEncoder_BGRA_RawBuildsRawvideoInput(t *testing.T) {
	e := &EncoderStage{
		StreamID_: "canvas-1",
		Media: MediaSource{
			Video: ComposerFrameSource{
				Socket: "/tmp/vn-bus-composer-canvas-1.sock",
				Width:  3840, Height: 1080, Fps: 30,
			},
		},
		Cfg:       EncoderConfig{Codec: "h265", Bitrate: "12M"},
		Publish:   []PublishTarget{{Type: "rtsp", URL: "rtsp://localhost:8554/canvas-1"}},
		VNSinkBin: "/usr/local/bin/vn-sink",
	}
	argv, _, err := e.Command()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	cmd := argv[2]
	if !strings.Contains(cmd, "-f rawvideo") || !strings.Contains(cmd, "-pix_fmt bgra") {
		t.Errorf("BGRA raw input missing in: %s", cmd)
	}
	if !strings.Contains(cmd, "-s 3840x1080") {
		t.Errorf("dims missing in: %s", cmd)
	}
	if !strings.Contains(cmd, "-framerate 30") {
		t.Errorf("framerate missing in: %s", cmd)
	}
	if !strings.Contains(cmd, "-c:v hevc_rkmpp") {
		t.Errorf("expected hevc_rkmpp encoder in: %s", cmd)
	}
}

func TestEncoder_CustomEncoderArgsReplacesEncoderTail(t *testing.T) {
	e := &EncoderStage{
		StreamID_: "cam",
		Media: MediaSource{
			Video: ProducerFrameSource{Socket: "/tmp/sock"},
		},
		CustomEncoderArgs: "-c:v libx264 -preset ultrafast -f rtsp rtsp://localhost:8554/cam",
		Publish:           []PublishTarget{{Type: "rtsp", URL: "ignored-because-custom"}},
		VNSinkBin:         "/usr/bin/vn-sink",
	}
	argv, _, err := e.Command()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	cmd := argv[2]
	if !strings.Contains(cmd, "-c:v libx264") {
		t.Errorf("custom encoder not honored: %s", cmd)
	}
	if strings.Contains(cmd, "h264_rkmpp") {
		t.Errorf("default encoder leaked through despite custom args: %s", cmd)
	}
	// Daemon-owned input fragment still present (custom args don't touch input).
	if !strings.Contains(cmd, "vn-sink --socket /tmp/sock") {
		t.Errorf("input fragment dropped: %s", cmd)
	}
}

func TestEncoder_AudioInputArgsAppended(t *testing.T) {
	e := &EncoderStage{
		StreamID_: "cam",
		Media: MediaSource{
			Video: ProducerFrameSource{Socket: "/tmp/sock"},
			Audio: ALSADirectAudio{Config: AudioConfig{Devices: []string{"hw:0", "hw:1"}}},
		},
		Cfg:       EncoderConfig{Codec: "h264", Bitrate: "2M"},
		Publish:   []PublishTarget{{Type: "rtsp", URL: "rtsp://x/y"}},
		VNSinkBin: "/usr/bin/vn-sink",
	}
	argv, _, _ := e.Command()
	cmd := argv[2]
	if !strings.Contains(cmd, "-f alsa -i hw:0") || !strings.Contains(cmd, "-f alsa -i hw:1") {
		t.Errorf("audio inputs missing in: %s", cmd)
	}
}

func TestEncoder_RejectsMissingMedia(t *testing.T) {
	e := &EncoderStage{StreamID_: "x", VNSinkBin: "/usr/bin/vn-sink"}
	if _, _, err := e.Command(); err == nil {
		t.Error("expected error for nil video media")
	}
}

func TestEncoder_RejectsEmptyPublish(t *testing.T) {
	e := &EncoderStage{
		StreamID_: "x",
		Media:     MediaSource{Video: ProducerFrameSource{Socket: "/tmp/x"}},
		VNSinkBin: "/usr/bin/vn-sink",
	}
	if _, _, err := e.Command(); err == nil {
		t.Error("expected error for empty publish targets")
	}
}

func TestEncoder_ReconfigureRequiresRestart(t *testing.T) {
	e := &EncoderStage{}
	if err := e.Reconfigure(nil); err != ErrRequiresRestart {
		t.Errorf("expected ErrRequiresRestart, got %v", err)
	}
}

func TestEncoder_KindAndID(t *testing.T) {
	e := &EncoderStage{StreamID_: "abc"}
	if e.Kind() != KindEncoder {
		t.Errorf("Kind = %v, want KindEncoder", e.Kind())
	}
	if e.ID() != "encoder:abc" {
		t.Errorf("ID = %s, want encoder:abc", e.ID())
	}
}

func TestProducer_CommandWithGrpc(t *testing.T) {
	p := &ProducerStage{
		DeviceID:   "hdmi0",
		DevicePath: "/dev/video0",
		BinaryPath: "/usr/bin/videonode-source",
		GrpcUds:    "/tmp/videonode-native/source-hdmi0.sock",
	}
	argv, _, err := p.Command()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	want := []string{
		"/usr/bin/videonode-source",
		"--device", "/dev/video0",
		"--out-socket", "/tmp/vn-bus-hdmi0.sock",
		"--grpc-listen", "/tmp/videonode-native/source-hdmi0.sock",
		"--device-id", "hdmi0",
	}
	if !equal(argv, want) {
		t.Errorf("argv mismatch.\n got: %v\nwant: %v", argv, want)
	}
}

func TestProducer_CommandWithoutGrpc(t *testing.T) {
	p := &ProducerStage{
		DeviceID:   "hdmi0",
		DevicePath: "/dev/video0",
		BinaryPath: "/usr/bin/videonode-source",
	}
	argv, _, _ := p.Command()
	for _, a := range argv {
		if a == "--grpc-listen" || a == "--device-id" {
			t.Errorf("standalone producer should not have control-plane flags: %v", argv)
		}
	}
}

func TestProducer_KindAndPoolKey(t *testing.T) {
	p := &ProducerStage{DeviceID: "cam-1"}
	if p.Kind() != KindProducer {
		t.Errorf("Kind = %v", p.Kind())
	}
	if p.ID() != "producer:cam-1" {
		t.Errorf("ID = %s", p.ID())
	}
	if p.StreamID() != "" {
		t.Errorf("StreamID should be empty for producer, got %s", p.StreamID())
	}
}

func TestComposer_Command(t *testing.T) {
	c := &ComposerStage{
		StreamID_:  "canvas-1",
		BinaryPath: "/usr/bin/videonode-composer",
		DRMDevice:  "/dev/dri/renderD128",
		CanvasFPS:  30,
		GrpcUds:    "/tmp/videonode-native/composer-canvas-1-composer.sock",
	}
	argv, _, err := c.Command()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	want := []string{
		"/usr/bin/videonode-composer",
		"--drm-device", "/dev/dri/renderD128",
		"--grpc-listen", "/tmp/videonode-native/composer-canvas-1-composer.sock",
		"--composer-id", "canvas-1-composer",
		"--scm-out", "/tmp/vn-bus-composer-canvas-1.sock",
		"--target-fps", "30",
	}
	if !equal(argv, want) {
		t.Errorf("argv mismatch.\n got: %v\nwant: %v", argv, want)
	}
}

func TestComposer_RequiresGrpc(t *testing.T) {
	c := &ComposerStage{
		StreamID_:  "x",
		BinaryPath: "/bin",
		DRMDevice:  "/dev/dri/renderD128",
	}
	if _, _, err := c.Command(); err == nil {
		t.Error("composer without GrpcUds should fail")
	}
}

func TestComposer_KindAndPoolKey(t *testing.T) {
	c := &ComposerStage{StreamID_: "x"}
	if c.Kind() != KindComposer {
		t.Errorf("Kind = %v", c.Kind())
	}
	if c.ID() != "composer:x" {
		t.Errorf("ID = %s", c.ID())
	}
}

func TestNeedsComposer(t *testing.T) {
	tests := []struct {
		name string
		s    Stream
		want bool
	}{
		{"empty inputs", Stream{}, false},
		{"single input no effects", Stream{Inputs: []InputRef{{ID: "a"}}}, false},
		{"two inputs", Stream{Inputs: []InputRef{{ID: "a"}, {ID: "b"}}}, true},
		{
			"single input with perspective effect",
			Stream{
				Inputs:  []InputRef{{ID: "a"}},
				Effects: map[string][]Effect{"a": {{Type: "perspective"}}},
			},
			true,
		},
		{
			"single input with empty effect list — does not engage",
			Stream{
				Inputs:  []InputRef{{ID: "a"}},
				Effects: map[string][]Effect{"a": {}},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsComposer(tt.s); got != tt.want {
				t.Errorf("NeedsComposer = %v, want %v", got, tt.want)
			}
		})
	}
}
