package config

import "regexp"

var melangeVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// ValidMelangeVersion reports whether a version is safe for Melange placeholder substitution.
func ValidMelangeVersion(version string) bool {
	return melangeVersionPattern.MatchString(version)
}

// MelangeFor returns the Melange configuration for one image version and type.
// An exact version-scoped entry takes precedence over the shared type template.
func (d *ImageDef) MelangeFor(version, typeName string) *MelangeSpec {
	if d == nil {
		return nil
	}
	if meta, ok := d.Versions[version]; ok {
		if spec, configured := meta.Melange[typeName]; configured {
			return spec
		}
	}
	tmpl, ok := d.Types[typeName]
	if !ok {
		return nil
	}
	return tmpl.Melange
}
