package ffmpeg

import "strings"

// ShellQuote returns s safe to embed in a /bin/sh -c command line. Strings
// free of shell-special characters pass through unquoted; the rest are
// wrapped in single quotes with embedded single quotes escaped.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\r'\"\\$`|&;<>(){}[]*?#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
