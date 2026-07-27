package buildmetadata

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrent_reports_stable_development_metadata_without_linker_injection(t *testing.T) {
	// Given a normal non-release test binary.

	// When its build metadata is read.
	info := Current()

	// Then the public version is development and every machine field is populated.
	assert.Equal(t, DevelopmentVersion, info.Version)
	assert.Equal(t, UnknownValue, info.SourceSHA)
	assert.Equal(t, UnknownValue, info.BuildKey)
	assert.NotEmpty(t, info.GoVersion)
	assert.Equal(t, runtime.GOOS, info.GOOS)
	assert.Equal(t, runtime.GOARCH, info.GOARCH)
	assert.NotEmpty(t, info.CGOEnabled)
	assert.NotEmpty(t, info.VCSStatus)
	if info.VCSStatus == UnknownVCSStatus {
		assert.Nil(t, info.Dirty)
	}
	assert.Equal(t, DevelopmentStatus, info.BuildStatus)
}

func TestMarshalInfo_emits_canonical_single_line_JSON(t *testing.T) {
	// Given complete version metadata.
	info := Info{
		Version:     DevelopmentVersion,
		SourceSHA:   "0123456789012345678901234567890123456789",
		BuildKey:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		GoVersion:   "go1.26.5",
		Dirty:       new(false),
		BuildStatus: BuiltStatus,
	}

	// When it is marshaled for the CLI.
	data, err := MarshalInfo(info)

	// Then it is one canonical JSON object terminated by one newline.
	require.NoError(t, err)
	assert.Equal(t, "{\"version\":\"dev\",\"source_sha\":\"0123456789012345678901234567890123456789\",\"build_key\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"go_version\":\"go1.26.5\",\"dirty\":false,\"build_status\":\"built\"}\n", string(data))
	var decoded Info
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, info, decoded)
}
