package led

import (
	"log/slog"
	"os"
	"testing"
)

func TestNoopController(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctrl := newNoop(logger)

	// Should return no errors
	if err := ctrl.Set("user", true, "solid"); err != nil {
		t.Errorf("Set() returned error: %v", err)
	}

	// Should return empty lists
	if types := ctrl.Available(); len(types) != 0 {
		t.Errorf("Available() = %v, want empty slice", types)
	}

	if patterns := ctrl.Patterns(); len(patterns) != 0 {
		t.Errorf("Patterns() = %v, want empty slice", patterns)
	}
}

func TestSysfsController_Available(t *testing.T) {
	tests := []struct {
		name     string
		leds     map[string]string
		wantLen  int
		contains string
	}{
		{
			name:     "NanoPC-T6 LEDs",
			leds:     map[string]string{"user": "usr_led", "system": "sys_led"},
			wantLen:  2,
			contains: "user",
		},
		{
			name:     "Orange Pi LEDs",
			leds:     map[string]string{"blue": "blue_led", "green": "green_led"},
			wantLen:  2,
			contains: "blue",
		},
		{
			name:    "No LEDs",
			leds:    map[string]string{},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := newSysfs(tt.leds)
			available := ctrl.Available()

			if len(available) != tt.wantLen {
				t.Errorf("Available() len = %d, want %d", len(available), tt.wantLen)
			}

			if tt.contains != "" {
				found := false
				for _, ledType := range available {
					if ledType == tt.contains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Available() does not contain %q", tt.contains)
				}
			}
		})
	}
}

func TestSysfsController_Patterns(t *testing.T) {
	ctrl := newSysfs(map[string]string{"user": "usr_led"})
	patterns := ctrl.Patterns()

	expectedPatterns := []string{"solid", "blink", "heartbeat"}
	if len(patterns) != len(expectedPatterns) {
		t.Errorf("Patterns() len = %d, want %d", len(patterns), len(expectedPatterns))
	}

	for _, expected := range expectedPatterns {
		found := false
		for _, pattern := range patterns {
			if pattern == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Patterns() missing %q", expected)
		}
	}
}

func TestSysfsController_Set_InvalidType(t *testing.T) {
	ctrl := newSysfs(map[string]string{"user": "usr_led"})

	// Should error on unsupported LED type
	err := ctrl.Set("nonexistent", true, "")
	if err == nil {
		t.Error("Set() with invalid LED type should return error")
	}
}