package led

import (
	"log/slog"
	"os"
	"testing"
)

func TestNew(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Should always return a non-nil controller
	ctrl := New(logger)
	if ctrl == nil {
		t.Fatal("New() returned nil")
	}

	// Should return valid interface methods
	available := ctrl.Available()
	patterns := ctrl.Patterns()

	// Results should be non-nil slices (even if empty)
	if available == nil {
		t.Error("Available() returned nil")
	}
	if patterns == nil {
		t.Error("Patterns() returned nil")
	}

	// Set should not panic
	_ = ctrl.Set("user", true, "solid")
}

func TestDetectBoard(t *testing.T) {
	model := detectBoard()

	// Should return a non-empty string (or "unknown")
	if model == "" {
		t.Error("detectBoard() returned empty string")
	}

	// Should handle missing file gracefully
	if model == "unknown" {
		t.Log("Board model unknown (expected on non-SBC systems)")
	}
}