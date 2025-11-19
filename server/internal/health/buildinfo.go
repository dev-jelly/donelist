package health

import "time"

// BuildInfo contains build and version information
type BuildInfo struct {
	Version   string    `json:"version"`
	GitCommit string    `json:"git_commit,omitempty"`
	GitBranch string    `json:"git_branch,omitempty"`
	BuildTime string    `json:"build_time,omitempty"`
	GoVersion string    `json:"go_version,omitempty"`
	BuildHost string    `json:"build_host,omitempty"`
}

// These variables are meant to be set at build time using ldflags
var (
	// Version is the application version (e.g., "1.0.0")
	Version = "dev"
	// GitCommit is the git commit SHA
	GitCommit = "unknown"
	// GitBranch is the git branch name
	GitBranch = "unknown"
	// BuildTime is when the binary was built
	BuildTime = "unknown"
	// GoVersion is the Go version used to build
	GoVersion = "unknown"
	// BuildHost is the hostname where the build occurred
	BuildHost = "unknown"
)

// GetBuildInfo returns the current build information
func GetBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   Version,
		GitCommit: GitCommit,
		GitBranch: GitBranch,
		BuildTime: BuildTime,
		GoVersion: GoVersion,
		BuildHost: BuildHost,
	}
}

// UpdatedHealthReport extends HealthReport with build information
type UpdatedHealthReport struct {
	Status     Status                     `json:"status"`
	Version    string                     `json:"version"`
	BuildInfo  BuildInfo                  `json:"build_info"`
	Timestamp  time.Time                  `json:"timestamp"`
	Uptime     time.Duration              `json:"uptime_seconds"`
	Components map[string]ComponentHealth `json:"components"`
}
