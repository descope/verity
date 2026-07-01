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

func TestValidateFIPSProfile_acceptsOpenSSLWithProviderBase(t *testing.T) {
	def := &config.ImageDef{
		Name:     "nginx",
		Upstream: config.Upstream{Package: "nginx"},
		Types: map[string]config.TypeTemplate{
			"fips": {
				Base:        "wolfi-fips",
				FIPSProfile: config.FIPSProfileOpenSSL,
				Packages:    []string{"nginx", "openssl-provider-fips"},
				Entrypoint:  "/usr/bin/openssl-fips-entrypoint /usr/sbin/nginx",
				Environment: map[string]string{
					"OPENSSL_MODULES": "/usr/lib/ossl-modules",
					"OPENSSL_CONF":    "/etc/ssl/openssl-fips.cnf",
				},
				Melange: &config.MelangeSpec{Bespoke: "openssl-provider-fips.yaml"},
			},
		},
	}

	require.NoError(t, config.Validate(def))
}

func TestValidateFIPSProfile_rejectsOpenSSLWithoutProviderWiring(t *testing.T) {
	def := &config.ImageDef{
		Name:     "nginx",
		Upstream: config.Upstream{Package: "nginx"},
		Types: map[string]config.TypeTemplate{
			"fips": {Base: "wolfi-fips", FIPSProfile: config.FIPSProfileOpenSSL},
		},
	}

	err := config.Validate(def)
	require.ErrorIs(t, err, config.ErrUnsupportedFIPSProfile)
}

func TestValidateFIPSProfile_rejectsOpenSSLWrapperWithoutCommand(t *testing.T) {
	def := &config.ImageDef{
		Name:     "nginx",
		Upstream: config.Upstream{Package: "nginx"},
		Types: map[string]config.TypeTemplate{
			"fips": {
				Base:        "wolfi-fips",
				FIPSProfile: config.FIPSProfileOpenSSL,
				Packages:    []string{"nginx", "openssl-provider-fips"},
				Entrypoint:  "/usr/bin/openssl-fips-entrypoint ",
				Environment: map[string]string{
					"OPENSSL_MODULES": "/usr/lib/ossl-modules",
					"OPENSSL_CONF":    "/etc/ssl/openssl-fips.cnf",
				},
				Melange: &config.MelangeSpec{Bespoke: "openssl-provider-fips.yaml"},
			},
		},
	}

	err := config.Validate(def)
	require.ErrorIs(t, err, config.ErrMissingFIPSEnvironment)
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
