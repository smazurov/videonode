package pipeline

import (
	"strings"

	"github.com/smazurov/videonode/internal/ffmpeg"
)

// shellJoinArgv joins an argv slice into a single shell-quoted command
// string. Used by EncoderStage when composing `vn-sink | ffmpeg` and
// when building the daemon-owned input fragment that's prepended to a
// user's CustomEncoderArgs verbatim shell tail.
func shellJoinArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = ffmpeg.ShellQuote(a)
	}
	return strings.Join(parts, " ")
}
