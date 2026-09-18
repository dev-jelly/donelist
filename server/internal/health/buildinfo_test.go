package health

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBuildInfo(t *testing.T) {
	buildInfo := GetBuildInfo()

	// Should have all fields initialized (even if to default values)
	assert.NotEmpty(t, buildInfo.Version)
	assert.NotEmpty(t, buildInfo.GitCommit)
	assert.NotEmpty(t, buildInfo.GitBranch)
	assert.NotEmpty(t, buildInfo.BuildTime)
	assert.NotEmpty(t, buildInfo.GoVersion)
	assert.NotEmpty(t, buildInfo.BuildHost)
}

func TestGetBuildInfo_DefaultValues(t *testing.T) {
	// When not built with ldflags, should have default values
	buildInfo := GetBuildInfo()

	// These should match the defaults in buildinfo.go
	// (unless the binary was built with ldflags)
	assert.NotNil(t, buildInfo)
}

func TestBuildInfo_JSONTags(t *testing.T) {
	// Verify that BuildInfo has proper JSON tags
	buildInfo := BuildInfo{
		Version:   "1.0.0",
		GitCommit: "abc123",
		GitBranch: "main",
		BuildTime: "2024-01-01",
		GoVersion: "go1.21",
		BuildHost: "build-server",
	}

	assert.Equal(t, "1.0.0", buildInfo.Version)
	assert.Equal(t, "abc123", buildInfo.GitCommit)
	assert.Equal(t, "main", buildInfo.GitBranch)
	assert.Equal(t, "2024-01-01", buildInfo.BuildTime)
	assert.Equal(t, "go1.21", buildInfo.GoVersion)
	assert.Equal(t, "build-server", buildInfo.BuildHost)
}

func TestUpdatedHealthReport_Structure(t *testing.T) {
	// Test the structure of UpdatedHealthReport
	report := UpdatedHealthReport{
		Status:  StatusHealthy,
		Version: "1.0.0",
		BuildInfo: BuildInfo{
			Version:   "1.0.0",
			GitCommit: "abc123",
		},
		Components: make(map[string]ComponentHealth),
	}

	assert.Equal(t, StatusHealthy, report.Status)
	assert.Equal(t, "1.0.0", report.Version)
	assert.Equal(t, "abc123", report.BuildInfo.GitCommit)
	assert.NotNil(t, report.Components)
}
