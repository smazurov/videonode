package cmd

import (
	"fmt"
	"runtime/debug"
)

// VersionCmd prints version + VCS info captured at build time.
// Works with both `go build` and `go install` — no ldflags needed.
type VersionCmd struct{}

func (c *VersionCmd) Run(_ *Context) error {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Fprintln(stdout(), "testenv (build info unavailable)")
		return nil
	}
	commit, modified, t := "unknown", "", ""
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				modified = "-dirty"
			}
		case "vcs.time":
			t = s.Value
		}
	}
	if len(commit) > 12 {
		commit = commit[:12]
	}
	fmt.Fprintf(stdout(), "testenv %s%s (go %s, built %s)\n", commit, modified, info.GoVersion, t)
	return nil
}
