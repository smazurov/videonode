package version

import (
	"fmt"
	"runtime"
)

var (
	// These variables are set via ldflags during build
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
	BuildID   = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	BuildID   string `json:"build_id"`
	GoVersion string `json:"go_version"`
	Compiler  string `json:"compiler"`
	Platform  string `json:"platform"`
}

func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		BuildID:   BuildID,
		GoVersion: runtime.Version(),
		Compiler:  runtime.Compiler,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

func String() string {
	return Version
}
