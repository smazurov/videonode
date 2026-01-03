package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the application version, set via ldflags during build.
	Version = "dev"
	// GitCommit is the git commit hash, set via ldflags during build.
	GitCommit = "unknown"
	// BuildDate is the build timestamp, set via ldflags during build.
	BuildDate = "unknown"
	// BuildID is the build identifier, set via ldflags during build.
	BuildID = "unknown"
)

// Info contains version and build metadata.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	BuildID   string `json:"build_id"`
	GoVersion string `json:"go_version"`
	Compiler  string `json:"compiler"`
	Platform  string `json:"platform"`
}

// Get returns version and build information.
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

// String returns the application version string.
func String() string {
	return Version
}
