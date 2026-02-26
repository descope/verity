package internal

import "strings"

// Image represents a container image reference.
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
func sanitize(ref string) string {
	r := strings.NewReplacer("/", "_", ":", "_")
	return r.Replace(ref)
}
