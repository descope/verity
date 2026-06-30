package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/discovery"
)

const opensslFIPSConfig = "/etc/ssl/openssl-fips.cnf"

func TestRepositoryFIPSVariantsDeclareFIPSProfile(t *testing.T) {
	paths, err := filepath.Glob("../../../images/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, paths)

	for _, path := range paths {
		def, err := config.LoadImage(path)
		require.NoError(t, err)
		require.NoError(t, config.Validate(def))

		fips, ok := def.Types["fips"]
		if !ok {
			continue
		}

		require.Truef(t, fips.FIPSProfile.Valid(), "%s fips variant must declare a supported fips-profile", filepath.Base(path))

		switch fips.FIPSProfile {
		case config.FIPSProfileGo:
			require.Equalf(t, "wolfi-base", fips.Base, "%s Go FIPS uses Go's module, not OpenSSL runtime base", filepath.Base(path))
			require.Contains(t, fips.Environment["GODEBUG"], "fips140=on")
		case config.FIPSProfileOpenSSL:
			require.Equalf(t, "wolfi-fips", fips.Base, "%s OpenSSL FIPS must inherit wolfi-fips", filepath.Base(path))
		case config.FIPSProfileJava:
			require.Equalf(t, "wolfi-fips", fips.Base, "%s Java FIPS must inherit wolfi-fips", filepath.Base(path))
			require.Contains(t, fips.Environment["JAVA_TOOL_OPTIONS"], "java.security.properties")
		case config.FIPSProfileReview:
			continue
		}

		if def.Name == "node" {
			nodeOptions := fips.Environment["NODE_OPTIONS"]
			require.Contains(t, nodeOptions, "--openssl-config="+opensslFIPSConfig)
			require.Contains(t, nodeOptions, "--force-fips")
		}
		if def.Name == "golang" {
			require.Contains(t, fips.Environment["GOFIPS140"], "latest")
		}
	}
}

func TestRepositoryGo123DoesNotPublishFIPSVariant(t *testing.T) {
	def, err := config.LoadImage("../../../images/golang.yaml")
	require.NoError(t, err)

	require.True(t, discovery.ShouldSkipType(def, "1.23", "fips"))
	require.False(t, discovery.ShouldSkipType(def, "1.24", "fips"))
}
