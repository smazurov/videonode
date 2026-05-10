package ffmpeg

import "fmt"

// ParseResolution parses an "WxH" resolution string.
func ParseResolution(s string) (int, int, error) {
	if s == "" {
		return 0, 0, fmt.Errorf("empty resolution")
	}
	var w, h int
	if _, err := fmt.Sscanf(s, "%dx%d", &w, &h); err != nil {
		return 0, 0, fmt.Errorf("parse resolution %q: %w", s, err)
	}
	return w, h, nil
}
