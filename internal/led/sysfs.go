package led

import (
	"fmt"
	"os"
	"path/filepath"
)

const sysfsLEDPath = "/sys/class/leds"

// sysfs implements Controller using Linux sysfs LED interface
type sysfs struct {
	leds     map[string]string // LED type -> sysfs name mapping
	patterns []string          // Available trigger patterns
}

// newSysfs creates a new sysfs LED controller with board-specific LED mappings
func newSysfs(leds map[string]string) *sysfs {
	return &sysfs{
		leds: leds,
		patterns: []string{
			"none",        // Manual control
			"heartbeat",   // Blinking heartbeat pattern
			"default-on",  // Always on (solid)
		},
	}
}

// Set controls an LED's state and optional pattern
func (s *sysfs) Set(ledType string, enabled bool, pattern string) error {
	sysfsName, ok := s.leds[ledType]
	if !ok {
		return fmt.Errorf("LED type %q not supported on this board", ledType)
	}

	ledPath := filepath.Join(sysfsLEDPath, sysfsName)

	// Check if LED exists
	if _, err := os.Stat(ledPath); os.IsNotExist(err) {
		return fmt.Errorf("LED %q not found at %s", ledType, ledPath)
	}

	// Set pattern/trigger if specified
	if pattern != "" {
		triggerPath := filepath.Join(ledPath, "trigger")
		var triggerValue string

		switch pattern {
		case "solid":
			triggerValue = "default-on"
		case "blink":
			triggerValue = "heartbeat"
		case "heartbeat":
			triggerValue = "heartbeat"
		default:
			triggerValue = pattern // Allow raw trigger names
		}

		if err := os.WriteFile(triggerPath, []byte(triggerValue), 0644); err != nil {
			return fmt.Errorf("failed to set LED trigger: %w", err)
		}

		// If pattern is set, we often want the LED on
		// Set trigger to "none" first to allow manual control if needed
		if pattern == "solid" {
			if err := os.WriteFile(triggerPath, []byte("none"), 0644); err != nil {
				return fmt.Errorf("failed to set LED trigger to none: %w", err)
			}
		}
	}

	// Set brightness (on/off)
	brightnessPath := filepath.Join(ledPath, "brightness")
	var brightnessValue string
	if enabled {
		brightnessValue = "1"
	} else {
		brightnessValue = "0"
	}

	if err := os.WriteFile(brightnessPath, []byte(brightnessValue), 0644); err != nil {
		return fmt.Errorf("failed to set LED brightness: %w", err)
	}

	return nil
}

// Available returns the list of LED types supported by this controller
func (s *sysfs) Available() []string {
	types := make([]string, 0, len(s.leds))
	for ledType := range s.leds {
		types = append(types, ledType)
	}
	return types
}

// Patterns returns the list of patterns supported by this controller
func (s *sysfs) Patterns() []string {
	return []string{"solid", "blink", "heartbeat"}
}