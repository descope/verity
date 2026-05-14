package config

import "strings"

// versionPlaceholder is the literal string substituted with concrete version
// values when rendering apko configs. Mirrors apkindex.versionPlaceholder so
// the helper below can stay independent of the apkindex package.
const versionPlaceholder = "{{version}}"

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

// VersionedPackagePattern returns the package template that drives apk
// solving for this image. Precedence:
//
//  1. Upstream.Package if it contains "{{version}}" — the natural
//     resolution input for images like kyverno, cilium, crossplane (e.g.
//     "kyverno-{{version}}").
//  2. Otherwise, the first package across types[*].packages that
//     contains "{{version}}" — used by images like erlang and haproxy
//     that declare an unversioned upstream.package ("erlang") while
//     templating the version in the type's packages: list (e.g.
//     ["erlang-{{version}}"]). The apko constraint that needs alias
//     resolution comes from the type's packages: list, not from
//     upstream.package, so the helper looks there too.
//  3. "" when neither has the placeholder — image is either purely
//     unversioned (single "latest" build) or templates the version only
//     in non-packages fields (e.g. nginx puts {{version}} in
//     environment but has packages: ["nginx-mainline"]). No alias
//     resolution applies in that case.
//
// Type-template iteration order is non-deterministic (map traversal),
// but in practice all types of one image share the same versioned
// pattern (verity convention: a type adds package suffixes, not
// version-stem variants). When they diverge, the first one seen is
// used; this is documented behavior and tests assert on the simpler
// shape.
func (d *ImageDef) VersionedPackagePattern() string {
	if d == nil {
		return ""
	}
	if strings.Contains(d.Upstream.Package, versionPlaceholder) {
		return d.Upstream.Package
	}
	for _, tmpl := range d.Types {
		for _, pkg := range tmpl.Packages {
			if strings.Contains(pkg, versionPlaceholder) {
				return pkg
			}
		}
	}
	return ""
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
	Melange     *MelangeSpec      `yaml:"melange,omitempty"`
}

// MelangeSpec describes a custom melange package build that runs before apko
// publish. The built package is injected into the apko config as a local repo.
//
// Use Upstream to rebuild an existing Wolfi package with overrides (e.g. adding
// GOFIPS140 via EnvFile). Use Bespoke for fully custom melange YAMLs that don't
// exist upstream. Exactly one of Upstream or Bespoke must be set.
type MelangeSpec struct {
	// Upstream is the package key in packages/upstream.lock.json. The melange
	// YAML is fetched from wolfi-dev/os at the pinned commit.
	Upstream string `yaml:"upstream,omitempty"`

	// Bespoke is a filename (without path) in packages/bespoke/. Used for
	// packages that don't exist in Wolfi or need radical changes.
	Bespoke string `yaml:"bespoke,omitempty"`

	// EnvFile is a filename (without path) in packages/overrides/. Passed to
	// melange build via --env-file to inject build-time environment variables
	// (e.g. GOFIPS140=v1.0.0) without modifying the upstream YAML.
	EnvFile string `yaml:"env-file,omitempty"`

	// BuildOption is passed to melange build via --build-option. It selects a
	// named variant defined in the melange YAML's options: block.
	BuildOption string `yaml:"build-option,omitempty"`
}

// PathDef is one path entry in an apko config.
//
// For type: "directory" (the default), set Path/UID/GID/Permissions.
// For type: "symlink", set Path (the link location) and Source (the target
// the link points to). UID/GID/Permissions are accepted by apko for
// symlinks but the kernel ignores them at use time.
//
// Symlink entries are how chart-target rebuilds can satisfy chart Deployment
// templates that hardcode binaries at non-FHS paths (e.g. chart expects
// /usr/local/bin/etcd; verity's wolfi rebuild ships /usr/bin/etcd) without
// duplicating the binary or modifying the upstream wolfi melange recipe.
// See verity-org/verity#318 (A.2 FHS-path mismatch sub-cluster).
//
// Type validation surface: render.go validates only "directory" and "symlink"
// shapes. Any other Type value is forwarded to apko verbatim — used in this
// repo to set "hardened-binary" for the Bucket H image-perm fixes
// (SCR-2026-05-14-001 §2 AC-5), where the binary needs 0o755 with apko's
// hardened-binary marker rather than a plain directory entry. "hardened-binary"
// is apko's own standard value; verity treats it as opaque pass-through.
type PathDef struct {
	Path string `yaml:"path"`
	// Type defaults to "directory"; "symlink" is the only value verity
	// validates structurally. Other values (notably "hardened-binary") are
	// forwarded to apko unvalidated.
	Type        string `yaml:"type,omitempty"`
	Source      string `yaml:"source,omitempty"` // for type: symlink — the target path the link points to
	UID         int    `yaml:"uid"`
	GID         int    `yaml:"gid"`
	Permissions string `yaml:"permissions,omitempty"` // e.g. "0o755"
}

// VersionMeta holds human-curated metadata for a discovered version.
// Fields are optional — versions without an entry in the map are still
// built with auto-generated tags.
type VersionMeta struct {
	EOL       string   `yaml:"eol,omitempty"`        // "2027-04-30"
	Latest    bool     `yaml:"latest,omitempty"`     // carries the "latest" tag
	SkipTypes []string `yaml:"skip-types,omitempty"` // types to exclude for this version (e.g. ["fips"])
}
