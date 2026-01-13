package ffmpeg

import "strings"

// ParseLogLevel extracts the log level from ffmpeg output.
// FFmpeg with -loglevel level+info outputs lines like "[info] message"
// or "[component @ 0x...] [level] message" for component-specific logs.
// Returns the level and the message with level stripped but component preserved.
func ParseLogLevel(line string) (level, msg string) {
	if len(line) < 3 || line[0] != '[' {
		return "info", line
	}

	end := strings.Index(line, "] ")
	if end == -1 {
		return "info", line
	}

	bracket := line[1:end]

	if isLogLevel(bracket) {
		return bracket, line[end+2:]
	}

	// Check for component prefix: [component @ 0x...] [level] message
	// Keep the component, strip only the [level]
	component := line[:end+2]
	rest := line[end+2:]
	if len(rest) > 2 && rest[0] == '[' {
		if nextEnd := strings.Index(rest, "] "); nextEnd != -1 {
			nextBracket := rest[1:nextEnd]
			if isLogLevel(nextBracket) {
				return nextBracket, component + rest[nextEnd+2:]
			}
		}
	}

	return "info", line
}

func isLogLevel(s string) bool {
	switch s {
	case "quiet", "panic", "fatal", "error", "warning", "info", "verbose", "debug", "trace":
		return true
	}
	return false
}
