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
