package pipeline

import "strings"

// shellJoinArgv joins an argv slice into a single shell-quoted command
// string. Used by EncoderStage when composing `vn-sink | ffmpeg` and
// when building the daemon-owned input fragment that's prepended to a
// user's CustomEncoderArgs verbatim shell tail.
func shellJoinArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// shellQuote returns s safe to embed in a /bin/sh -c command line.
// Strings free of shell-special characters pass through unquoted; the
// rest are wrapped in single quotes with embedded single-quotes
// escaped via the standard '\” dance.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\r'\"\\$`|&;<>(){}[]*?#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
