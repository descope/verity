package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/config"
)

func TestImageDefMelangeForPrefersExactVersionAndPreservesSharedFallback(t *testing.T) {
	// Given: an existing shared config plus one exact override and one explicit disable.
	shared := &config.MelangeSpec{Bespoke: config.StringList{"shared.yaml"}}
	scoped := &config.MelangeSpec{Bespoke: config.StringList{"scoped.yaml"}}
	def := &config.ImageDef{
		Types: map[string]config.TypeTemplate{
			"default": {Melange: shared},
		},
		Versions: map[string]config.VersionMeta{
			"14": {Melange: map[string]*config.MelangeSpec{"default": scoped}},
			"16": {},
			"17": {Melange: map[string]*config.MelangeSpec{"default": nil}},
		},
	}

	// When: each configuration scope is resolved.
	// Then: exact entries win, absent entries fall back, and explicit nil disables the shared config.
	assert.Same(t, scoped, def.MelangeFor("14", "default"))
	assert.Same(t, shared, def.MelangeFor("16", "default"))
	assert.Nil(t, def.MelangeFor("17", "default"))
	assert.Same(t, shared, def.MelangeFor("18", "default"))
}

func TestValidateAcceptsVersionScopedMelange(t *testing.T) {
	// Given: a declared version with a Melange override for an existing type.
	def := &config.ImageDef{
		Name:     "postgres",
		Upstream: config.Upstream{Package: "postgresql-{{version}}"},
		Types: map[string]config.TypeTemplate{
			"default": {Base: "wolfi-base"},
		},
		Versions: map[string]config.VersionMeta{
			"14": {
				Melange: map[string]*config.MelangeSpec{
					"default": {Bespoke: config.StringList{"postgresql-14.yaml"}},
				},
			},
		},
	}

	// When: the image definition is validated.
	err := config.Validate(def)

	// Then: the scoped configuration is accepted.
	require.NoError(t, err)
}

func TestValidateRejectsVersionScopedMelangeForUndefinedType(t *testing.T) {
	// Given: a version-scoped Melange key that does not name a declared type.
	def := &config.ImageDef{
		Name:     "postgres",
		Upstream: config.Upstream{Package: "postgresql-{{version}}"},
		Types: map[string]config.TypeTemplate{
			"default": {Base: "wolfi-base"},
		},
		Versions: map[string]config.VersionMeta{
			"14": {
				Melange: map[string]*config.MelangeSpec{
					"typo": {Bespoke: config.StringList{"postgresql-14.yaml"}},
				},
			},
		},
	}

	// When: the image definition is validated.
	err := config.Validate(def)

	// Then: the invalid type reference is rejected with a typed error.
	require.ErrorIs(t, err, config.ErrMelangeTypeNotFound)
}
