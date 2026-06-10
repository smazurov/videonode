package pipeline

import (
	"slices"
	"testing"
)

func TestProducerStage_Command_TestMode_OmitsDevice(t *testing.T) {
	p := &ProducerStage{
		SourceID:   "src1",
		TestMode:   true,
		BinaryPath: "/bin/videonode-source",
	}
	argv, _, err := p.Command()
	if err != nil {
		t.Fatalf("Command() err = %v", err)
	}
	if slices.Contains(argv, "--test-pattern") {
		t.Errorf("argv must not contain --test-pattern (videonode-source has no such flag): %v", argv)
	}
	if slices.Contains(argv, "--device") {
		t.Errorf("test-mode argv must not pass --device: %v", argv)
	}
}

func TestProducerStage_Command_Pipe(t *testing.T) {
	p := &ProducerStage{
		SourceID:   "src1",
		PipeCmd:    "ffmpeg -i clip.mp4 -f yuv4mpegpipe -",
		BinaryPath: "/bin/videonode-source",
	}
	argv, _, err := p.Command()
	if err != nil {
		t.Fatalf("Command() err = %v", err)
	}
	idx := slices.Index(argv, "--pipe-cmd")
	if idx < 0 || idx+1 >= len(argv) || argv[idx+1] != p.PipeCmd {
		t.Errorf("expected --pipe-cmd %q in argv, got %v", p.PipeCmd, argv)
	}
	if slices.Contains(argv, "--device") {
		t.Errorf("pipe argv must not pass --device: %v", argv)
	}
}

func TestProducerStage_Command_ExactlyOneMode(t *testing.T) {
	tests := []struct {
		name  string
		stage ProducerStage
	}{
		{"none set", ProducerStage{SourceID: "s", BinaryPath: "/bin/x"}},
		{"device and test mode", ProducerStage{SourceID: "s", BinaryPath: "/bin/x", DevicePath: "/dev/video0", TestMode: true}},
		{"device and pipe", ProducerStage{SourceID: "s", BinaryPath: "/bin/x", DevicePath: "/dev/video0", PipeCmd: "cmd"}},
		{"test mode and pipe", ProducerStage{SourceID: "s", BinaryPath: "/bin/x", TestMode: true, PipeCmd: "cmd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := tt.stage.Command(); err == nil {
				t.Errorf("Command() expected error for %s", tt.name)
			}
		})
	}
}

func TestProducerStage_Command_Device(t *testing.T) {
	p := &ProducerStage{
		SourceID:   "src1",
		DevicePath: "/dev/video0",
		BinaryPath: "/bin/videonode-source",
	}
	argv, _, err := p.Command()
	if err != nil {
		t.Fatalf("Command() err = %v", err)
	}
	idx := slices.Index(argv, "--device")
	if idx < 0 || idx+1 >= len(argv) || argv[idx+1] != "/dev/video0" {
		t.Errorf("expected --device /dev/video0 in argv, got %v", argv)
	}
}
