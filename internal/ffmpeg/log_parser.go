package ffmpeg

import "strings"

// ParseLogLevel extracts the level from "[level] msg" or "[component] [level] msg" ffmpeg output.
// Also recognizes the glog/absl-style "LEVEL: msg" prefix that libraries like
// absl::ParseCommandLine emit on stderr without going through vn::log.
func ParseLogLevel(line string) (level, msg string) {
	if len(line) < 3 || line[0] != '[' {
		if lvl, rest, ok := parseGlogPrefix(line); ok {
			return lvl, rest
		}
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

	// "[component] [level] msg": keep component, strip [level].
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

// parseGlogPrefix matches glog/absl-style "LEVEL: msg" output (e.g. the
// "ERROR: Unknown command line flag ..." line absl prints when flag parsing
// fails). Returns the mapped slog level, the message without the prefix,
// and ok=true on a match.
func parseGlogPrefix(line string) (level, msg string, ok bool) {
	colon := strings.Index(line, ": ")
	if colon <= 0 {
		return "", "", false
	}
	tag := line[:colon]
	for i := 0; i < len(tag); i++ {
		if tag[i] < 'A' || tag[i] > 'Z' {
			return "", "", false
		}
	}
	switch tag {
	case "FATAL":
		return "fatal", line[colon+2:], true
	case "ERROR":
		return "error", line[colon+2:], true
	case "WARNING", "WARN":
		return "warning", line[colon+2:], true
	case "INFO":
		return "info", line[colon+2:], true
	}
	return "", "", false
}

func isLogLevel(s string) bool {
	switch s {
	case "quiet", "panic", "fatal", "error", "warning", "info", "verbose", "debug", "trace":
		return true
	}
	return false
}
