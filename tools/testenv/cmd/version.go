package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Version is the build-time version stamp set via -ldflags:
//
//	go install -ldflags "-X github.com/smazurov/videonode/tools/testenv/cmd.Version=$(git rev-parse --short HEAD)" .
var Version string

// VersionCmd prints version + VCS info.
type VersionCmd struct{}

// Run prints the version and VCS build info to stdout.
func (c *VersionCmd) Run(_ *Context) error {
	v := Version
	if v == "" {
		v = vcsRevision()
	}
	if v == "" {
		v = "dev"
	}
	_, _ = fmt.Fprintf(stdout(), "testenv %s (%s)\n", v, runtime.Version())
	return nil
}

func vcsRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	rev, dirty := "", ""
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	return rev + dirty
}
