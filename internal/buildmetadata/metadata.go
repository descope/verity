package buildmetadata

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	// DevelopmentVersion is used when no release version was injected.
	DevelopmentVersion = "dev"
	// UnknownValue marks metadata that was not available at build time.
	UnknownValue = "unknown"
)

// BuildStatus describes whether trusted release identity was supplied.
type BuildStatus string

const (
	// DevelopmentStatus describes a local or incomplete build.
	DevelopmentStatus BuildStatus = "development"
	// BuiltStatus describes a build with a valid source and build identity.
	BuiltStatus BuildStatus = "built"
)

// VCSStatus describes the source tree state recorded by Go's build info.
type VCSStatus string

const (
	// UnknownVCSStatus means Go did not record a source tree state.
	UnknownVCSStatus VCSStatus = "unknown"
	// CleanVCSStatus means Go recorded an unmodified source tree.
	CleanVCSStatus VCSStatus = "clean"
	// DirtyVCSStatus means Go recorded local source modifications.
	DirtyVCSStatus VCSStatus = "dirty"
)

// ErrInvalidMetadata reports malformed linker-injected identity.
var ErrInvalidMetadata = errors.New("invalid build metadata")

// These strings are the linker injection seam. Keep them simple for -X.
var (
	version    = DevelopmentVersion
	sourceSHA  = UnknownValue
	buildKey   = UnknownValue
	buildFlags = ""
)

// Info is the typed build and version metadata exposed by Verity.
type Info struct {
	Version     string      `json:"version"`
	SourceSHA   string      `json:"source_sha"`
	BuildKey    string      `json:"build_key"`
	GoVersion   string      `json:"go_version"`
	GOOS        string      `json:"goos,omitempty"`
	GOARCH      string      `json:"goarch,omitempty"`
	CGOEnabled  string      `json:"cgo_enabled,omitempty"`
	BuildFlags  []string    `json:"build_flags,omitempty"`
	Dirty       *bool       `json:"dirty"`
	VCSStatus   VCSStatus   `json:"vcs_status,omitempty"`
	BuildStatus BuildStatus `json:"build_status"`
}

// Current returns safe metadata, falling back to development values if an
// injected identity is malformed.
func Current() Info {
	details := runtimeSettings()
	info, err := resolveCurrent(&details)
	if err == nil {
		return info
	}

	return infoFromRuntime(resolvedIdentity{
		Version: DevelopmentVersion, SourceSHA: UnknownValue, BuildKey: UnknownValue,
		BuildStatus: DevelopmentStatus,
	}, &details)
}

// CurrentValidated returns metadata only when injected identity is trusted.
func CurrentValidated() (Info, error) {
	details := runtimeSettings()
	return resolveCurrent(&details)
}

// MarshalInfo emits one deterministic JSON object followed by one newline.
//
//nolint:gocritic // Keep the value parameter as the stable serialization API.
func MarshalInfo(info Info) ([]byte, error) {
	flags, err := canonicalBuildFlags(info.BuildFlags)
	if err != nil {
		return nil, err
	}
	info.BuildFlags = flags
	data, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("marshal build metadata: %w", err)
	}
	return append(data, '\n'), nil
}

func infoFromRuntime(identity resolvedIdentity, details *runtimeDetails) Info {
	return Info{
		Version:     identity.Version,
		SourceSHA:   identity.SourceSHA,
		BuildKey:    identity.BuildKey,
		GoVersion:   details.GoVersion,
		GOOS:        details.GOOS,
		GOARCH:      details.GOARCH,
		CGOEnabled:  details.CGOEnabled,
		BuildFlags:  details.BuildFlags,
		Dirty:       details.Dirty,
		VCSStatus:   details.VCSStatus,
		BuildStatus: identity.BuildStatus,
	}
}
