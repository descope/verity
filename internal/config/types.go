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
type VerityConfig struct {
	Overrides map[string]Override `yaml:"overrides,omitempty"`
}

// ImageSpec describes a single image to patch.
type ImageSpec struct {
	Name      string      `yaml:"name"`
	Image     string      `yaml:"image"`
	Tags      TagStrategy `yaml:"tags"`
	Target    TargetSpec  `yaml:"target,omitempty"`
	Platforms []string    `yaml:"platforms,omitempty"`
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
type Override struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}
