package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/discovery"
)

const opensslFIPSConfig = "/etc/ssl/openssl-fips.cnf"

func TestRepositoryFIPSVariantsEnableRuntimeFIPSByDefault(t *testing.T) {
	paths, err := filepath.Glob("../../../images/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, paths)

	for _, path := range paths {
		def, err := config.LoadImage(path)
		require.NoError(t, err)

		fips, ok := def.Types["fips"]
		if !ok {
			continue
		}

		env := fips.Environment
		godebug := env["GODEBUG"]
		nodeOptions := env["NODE_OPTIONS"]
		hasGoFIPS := strings.Contains(godebug, "fips140=on")
		hasOpenSSLFIPS := env["OPENSSL_CONF"] == opensslFIPSConfig
		hasNodeFIPS := strings.Contains(nodeOptions, "--force-fips")

		require.Truef(
			t,
			hasGoFIPS || hasOpenSSLFIPS || hasNodeFIPS,
			"%s fips variant must explicitly enable runtime FIPS by default",
			filepath.Base(path),
		)

		if def.Name == "node" {
			require.Contains(t, nodeOptions, "--openssl-config="+opensslFIPSConfig)
			require.Contains(t, nodeOptions, "--force-fips")
		}
	}
}

func TestRepositoryGo123DoesNotPublishFIPSVariant(t *testing.T) {
	def, err := config.LoadImage("../../../images/golang.yaml")
	require.NoError(t, err)

	require.True(t, discovery.ShouldSkipType(def, "1.23", "fips"))
	require.False(t, discovery.ShouldSkipType(def, "1.24", "fips"))
}
