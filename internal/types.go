package internal

import "strings"

// Image represents a container image found in chart values.
type Image struct {
	Registry   string `yaml:"registry,omitempty"`
	Repository string `yaml:"repository"`
	Tag        string `yaml:"tag,omitempty"`
	Path       string `yaml:"path"`
}

// Reference returns the full image reference string.
func (img Image) Reference() string {
	ref := img.Repository
	if img.Registry != "" {
		ref = img.Registry + "/" + ref
	}
	if img.Tag != "" {
		ref = ref + ":" + img.Tag
	}
	return ref
}

// sanitize converts an image reference to a safe filename by replacing path and tag separators.
// This is used for report filenames and map keys where "." and "@" need to be preserved
// (e.g., "docker.io/library/nginx:1.25" → "docker.io_library_nginx_1.25").
// Note: Different from postprocess.sanitizeImageName which also replaces "@" and "."
// for artifact naming where full sanitization is needed.
func sanitize(ref string) string {
	r := strings.NewReplacer("/", "_", ":", "_")
	return r.Replace(ref)
}

// PatchOptions holds configuration for patching operations (legacy, kept for backward compat).
type PatchOptions struct {
	TargetRegistry string
	BuildKitAddr   string
	ReportDir      string
	WorkDir        string
}

// PatchResult holds the outcome of patching a single image (legacy, kept for backward compat).
type PatchResult struct {
	Original           Image
	Patched            Image
	VulnCount          int
	Skipped            bool
	SkipReason         string
	Error              error
	ReportPath         string
	UpstreamReportPath string
	OverriddenFrom     string
}

// ImageOverride specifies a tag replacement for images matching a repository (legacy, kept for backward compat).
type ImageOverride struct {
	Repository string
	From       string
	To         string
}

// Skip reason constants.
const (
	SkipReasonNoPatchResult = "no patch result"
	SkipReasonUpToDate      = "already up to date"
)
