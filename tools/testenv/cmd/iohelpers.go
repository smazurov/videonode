package cmd

import (
	"io"
	"os"
)

// stdout / stderr indirection lets tests capture output. Production
// callers see os.Stdout/os.Stderr by default.
var (
	stdoutW io.Writer = os.Stdout
	stderrW io.Writer = os.Stderr
)

func stdout() io.Writer { return stdoutW }
func stderr() io.Writer { return stderrW }
