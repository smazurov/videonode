package pipeline

import "strings"

// shellPipe joins two argv lists into a single `/bin/sh -c "A | B"`
// payload. Used by EncoderStage to wrap `vn-sink | ffmpeg` as one
// pool entry. Both argv are shell-quoted to survive paths with spaces
// or special chars.
func shellPipe(left, right []string) string {
	return shellJoinArgv(left) + " | " + shellJoinArgv(right)
}

func shellJoinArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\r'\"\\$`|&;<>(){}[]*?#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// splitShellWords is a whitespace-naive splitter used for parsing
// CustomEncoderArgs. Same contract as the legacy CustomFFmpegCommand:
// callers escape their own argv with embedded whitespace. For now we
// honor double-quoted runs as one word; nothing more sophisticated.
func splitShellWords(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
