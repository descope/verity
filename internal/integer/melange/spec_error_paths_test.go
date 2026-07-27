package melange

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_ResolveSpec_rejects_invalid_request_boundaries(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		version   string
		imageType string
		wantError error
	}{
		{name: "image", image: "Invalid/Image", version: "1.0", imageType: "default", wantError: errInvalidImage},
		{name: "type", image: "sentinel", version: "1.0", imageType: "Invalid", wantError: errInvalidImageType},
		{name: "version", image: "sentinel", version: "bad/version", imageType: "default", wantError: errInvalidVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := ResolveSpec(t.TempDir(), tt.image, tt.version, tt.imageType)

			// Then
			require.ErrorIs(t, err, tt.wantError)
		})
	}
}

func Test_ResolveSpec_reports_definition_type_version_and_skip_errors(t *testing.T) {
	// Given
	imagesDir := filepath.Join(t.TempDir(), "images")
	writeTestFile(t, filepath.Join(imagesDir, "sentinel.yaml"), `
name: sentinel
description: sentinel
upstream:
  package: sentinel-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["sentinel-{{version}}"]
    melange:
      upstream: sentinel-{{version}}
  fips:
    base: wolfi-fips
    packages: ["sentinel-{{version}}"]
    melange:
      upstream: sentinel-fips-{{version}}
versions:
  "1.0":
    skip-types: [fips]
`)

	tests := []struct {
		name      string
		imagesDir string
		version   string
		imageType string
		wantError error
	}{
		{name: "missing definition", imagesDir: filepath.Join(t.TempDir(), "missing"), version: "1.0", imageType: "default"},
		{name: "missing type", imagesDir: imagesDir, version: "1.0", imageType: "dev", wantError: errImageTypeNotFound},
		{name: "missing version", imagesDir: imagesDir, version: "2.0", imageType: "default", wantError: errImageVersionNotFound},
		{name: "skipped type", imagesDir: imagesDir, version: "1.0", imageType: "fips", wantError: errImageTypeSkipped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := ResolveSpec(tt.imagesDir, "sentinel", tt.version, tt.imageType)

			// Then
			if tt.wantError == nil {
				require.ErrorContains(t, err, "load image")
				return
			}
			require.ErrorIs(t, err, tt.wantError)
		})
	}
}

func Test_ResolveConfigSpec_rejects_source_and_identifier_boundaries(t *testing.T) {
	tests := []struct {
		name      string
		input     *intconfig.MelangeSpec
		wantError error
	}{
		{name: "empty source", input: &intconfig.MelangeSpec{}, wantError: errInvalidSpecSource},
		{
			name:      "both sources",
			input:     &intconfig.MelangeSpec{Upstream: "sentinel", Bespoke: intconfig.StringList{"sentinel.yaml"}},
			wantError: errInvalidSpecSource,
		},
		{name: "invalid upstream", input: &intconfig.MelangeSpec{Upstream: "bad/value"}, wantError: errInvalidUpstreamKey},
		{name: "invalid bespoke", input: &intconfig.MelangeSpec{Bespoke: intconfig.StringList{"bad/value.yaml"}}, wantError: errInvalidBespokeFilename},
		{name: "invalid env file", input: &intconfig.MelangeSpec{Upstream: "sentinel", EnvFile: "bad/value.env"}, wantError: errInvalidOptionalField},
		{name: "invalid build option", input: &intconfig.MelangeSpec{Upstream: "sentinel", BuildOption: "bad/value"}, wantError: errInvalidOptionalField},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := ResolveConfigSpec(tt.input, "1.0")

			// Then
			require.ErrorIs(t, err, tt.wantError)
		})
	}
}
