package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/config"
)

func TestValidate_rejectsGoFIPSProfileWithoutWolfiBase(t *testing.T) {
	def := &config.ImageDef{
		Name:     "caddy",
		Upstream: config.Upstream{Package: "caddy"},
		Types: map[string]config.TypeTemplate{
			"fips": {
				Base:        "wolfi-fips",
				FIPSProfile: config.FIPSProfileGo,
				Environment: map[string]string{"GODEBUG": "fips140=on"},
			},
		},
	}

	err := config.Validate(def)
	require.ErrorIs(t, err, config.ErrInvalidFIPSBase)
}

func TestValidateFIPSProfile_requiresPinnedGoFIPS140(t *testing.T) {
	// Given: Go FIPS profile with runtime toggle but no pinned module.
	def := &config.ImageDef{
		Name:     "caddy",
		Upstream: config.Upstream{Package: "caddy"},
		Types: map[string]config.TypeTemplate{
			"fips": {
				Base:        "wolfi-base",
				FIPSProfile: config.FIPSProfileGo,
				Environment: map[string]string{"GODEBUG": "fips140=on"},
			},
		},
	}

	// When: config is validated.
	err := config.Validate(def)

	// Then: unpinned Go FIPS module is rejected.
	require.Error(t, err)
}

func TestValidateFIPSProfile_rejectsOpenSSLWithoutProvider(t *testing.T) {
	// Given: OpenSSL FIPS profile only inherits wolfi-fips base.
	def := &config.ImageDef{
		Name:     "nginx",
		Upstream: config.Upstream{Package: "nginx"},
		Types: map[string]config.TypeTemplate{
			"fips": {
				Base:        "wolfi-fips",
				FIPSProfile: config.FIPSProfileOpenSSL,
			},
		},
	}

	// When: config is validated.
	err := config.Validate(def)

	// Then: profile without provider artifact is rejected.
	require.Error(t, err)
}

func TestValidateFIPSProfile_rejectsJavaWithoutProvider(t *testing.T) {
	// Given: Java FIPS profile has JVM config but no provider artifact.
	def := &config.ImageDef{
		Name:     "java-app",
		Upstream: config.Upstream{Package: "java-app"},
		Types: map[string]config.TypeTemplate{
			"fips": {
				Base:        "wolfi-fips",
				FIPSProfile: config.FIPSProfileJava,
				Environment: map[string]string{"JAVA_TOOL_OPTIONS": "-Djava.security.properties=/etc/java/fips.properties"},
			},
		},
	}

	// When: config is validated.
	err := config.Validate(def)

	// Then: profile without provider artifact is rejected.
	require.Error(t, err)
}

func TestValidateFIPSProfile_requiresProfileForFIPSClaimTypeNames(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
	}{
		{name: "suffix fips", typeName: "sdk-fips"},
		{name: "fips prefix", typeName: "fips-openssl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: type name claims FIPS without declaring fips-profile.
			def := &config.ImageDef{
				Name:     "sdk",
				Upstream: config.Upstream{Package: "sdk"},
				Types: map[string]config.TypeTemplate{
					tt.typeName: {Base: "wolfi-base"},
				},
			}

			// When: config is validated.
			err := config.Validate(def)

			// Then: every FIPS-claim type requires a profile.
			require.ErrorIs(t, err, config.ErrMissingFIPSProfile)
		})
	}
}
