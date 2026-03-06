package config

// IntegerConfig is the global integer.yaml configuration.
type IntegerConfig struct {
	Target   TargetSpec   `yaml:"target"`
	Defaults DefaultsSpec `yaml:"defaults"`
}

// TargetSpec describes the registry where built images are published.
type TargetSpec struct {
	Registry string `yaml:"registry"`
}

// DefaultsSpec holds project-wide defaults applied to all images.
type DefaultsSpec struct {
	Archs []string `yaml:"archs"`
}

// ImageDef is an images/<name>.yaml file. It defines the apko config
// template for each build type and the upstream package discovery pattern.
type ImageDef struct {
	Name        string                  `yaml:"name"`
	Description string                  `yaml:"description"`
	Upstream    Upstream                `yaml:"upstream"`
	Types       map[string]TypeTemplate `yaml:"types"`
	Versions    map[string]VersionMeta  `yaml:"versions,omitempty"`
}

// Upstream describes how to discover available versions from the Wolfi APKINDEX.
//
// If Package contains "{{version}}", versions are discovered by scanning the
// APKINDEX for all packages matching the prefix before "{{version}}" and
// extracting the trailing version stem. Example: package "nodejs-{{version}}"
// matches nodejs-20, nodejs-22, nodejs-24 → versions [20, 22, 24].
//
// If Package contains no "{{version}}", the package is unversioned and only
// a single "latest" version is built.
type Upstream struct {
	Package string `yaml:"package"`
}

// TypeTemplate is the apko config template for one build type (default, dev, fips, …).
// All string fields support the "{{version}}" placeholder, which is replaced with
// the concrete version string when rendering.
type TypeTemplate struct {
	// Base references a _base/*.yaml file by stem name (e.g. "wolfi-base",
	// "wolfi-dev", "wolfi-fips"). Rendered as an apko include: directive.
	Base        string            `yaml:"base"`
	Packages    []string          `yaml:"packages"`
	Entrypoint  string            `yaml:"entrypoint,omitempty"`
	WorkDir     string            `yaml:"work-dir,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Paths       []PathDef         `yaml:"paths,omitempty"`
}

// PathDef is one path entry in an apko config.
type PathDef struct {
	Path        string `yaml:"path"`
	Type        string `yaml:"type,omitempty"` // defaults to "directory"
	UID         int    `yaml:"uid"`
	GID         int    `yaml:"gid"`
	Permissions string `yaml:"permissions,omitempty"` // e.g. "0o755"
}

// VersionMeta holds human-curated metadata for a discovered version.
// Fields are optional — versions without an entry in the map are still
// built with auto-generated tags.
type VersionMeta struct {
	EOL    string `yaml:"eol,omitempty"`    // "2027-04-30"
	Latest bool   `yaml:"latest,omitempty"` // carries the "latest" tag
}
