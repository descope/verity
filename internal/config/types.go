package config

// CopaConfig represents the copa-config.yaml structure.
type CopaConfig struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Target     TargetSpec          `yaml:"target,omitempty"`
	Charts     []ChartSpec         `yaml:"charts,omitempty"`
	Images     []ImageSpec         `yaml:"images"`
	Overrides  map[string]Override `yaml:"overrides,omitempty"` // deprecated: use verity.yaml
}

// VerityConfig represents verity.yaml — verity-specific settings that belong
// neither in Copa's copa-config.yaml nor in the standard Helm Chart.yaml.
//
// UnpatchableImages lists image names that should be skipped by the
// orchestrator regardless of source (standalone copa-config.yaml entry or
// chart-discovered). Names use the post-repoPath shape — registry stripped,
// no tag — e.g. "docker/library/redis". Reserve this list for images that
// have NO Wolfi/Integer rebuild AND no replacement we control: typically
// upstream chart references to registries/tags we no longer publish (such
// as argo-cd's hard-coded redis after the redis→valkey migration in #227).
// For distroless images without a rebuild, the preferred path is to author
// the rebuild stub at images/<name>.yaml and let --exclude-names handle
// the chart-discovery skip.
type VerityConfig struct {
	Overrides         map[string]Override    `yaml:"overrides,omitempty"`
	Replacements      map[string]Replacement `yaml:"replacements,omitempty"`
	UnpatchableImages []string               `yaml:"unpatchableImages,omitempty"`
}

// Replacement maps a chart-discovered image to a Verity Integer (Wolfi) image.
// The key in the map is a substring that matches the upstream image ref's repo path.
type Replacement struct {
	Registry string `yaml:"registry"`
	Image    string `yaml:"image"`
	Tag      string `yaml:"tag,omitempty"` // optional; defaults to upstream chart tag
}

// ImageSpec describes a single image to patch.
type ImageSpec struct {
	Name           string      `yaml:"name"`
	Image          string      `yaml:"image"`
	Tags           TagStrategy `yaml:"tags"`
	Target         TargetSpec  `yaml:"target,omitempty"`
	Platforms      []string    `yaml:"platforms,omitempty"`
	GoVcsURL       string      `yaml:"goVcsUrl,omitempty"`
	GoVcsTagPrefix string      `yaml:"goVcsTagPrefix,omitempty"`
}

// TargetSpec describes where to push the patched image.
type TargetSpec struct {
	Registry string `yaml:"registry,omitempty"`
	Tag      string `yaml:"tag,omitempty"`
}

// TagStrategy controls how tags are discovered for an image.
type TagStrategy struct {
	Strategy string   `yaml:"strategy"`
	Pattern  string   `yaml:"pattern,omitempty"`
	MaxTags  int      `yaml:"maxTags,omitempty"`
	List     []string `yaml:"list,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty"`
}

// ChartSpec describes a Helm chart from which to extract images.
// Field names match Helm's Chart.yaml dependencies format so the same struct
// can parse both copa-config.yaml and a standard Chart.yaml dependencies list.
type ChartSpec struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
}

// HelmChartFile represents a minimal Helm Chart.yaml, used only for reading
// the dependencies list. All other Chart.yaml fields are ignored.
type HelmChartFile struct {
	Dependencies []ChartSpec `yaml:"dependencies"`
}

// Override describes a tag variant substitution for chart images.
// If an image ref contains the map key and its tag contains the From suffix,
// the suffix is replaced with To (e.g., distroless-libc → debian).
// ValuePath optionally provides the dot-delimited values.yaml path for chart-gen
// (e.g., "image" → "image.repository" / "image.tag"). When set, chart-gen uses
// this path instead of auto-detecting from the chart's values.yaml tree.
type Override struct {
	From      string `yaml:"from"`
	To        string `yaml:"to"`
	ValuePath string `yaml:"valuePath,omitempty"`
}
