package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func TestOpenSearchDashboardsImageUsesConfigScrubbingEntrypoint(t *testing.T) {
	repo := ".."
	def, err := intconfig.LoadImage(filepath.Join(repo, "images", "opensearch-dashboards.yaml"))
	require.NoError(t, err)

	tmpl := def.Types["default"]
	assert.Equal(t, "/usr/local/bin/verity-opensearch-dashboards-entrypoint", tmpl.Entrypoint)
	assert.True(t, slices.Contains(tmpl.Packages, "verity-opensearch-dashboards-config"))
	require.NotNil(t, tmpl.Melange)
	assert.Equal(t, "verity-opensearch-dashboards-config.yaml", tmpl.Melange.Bespoke)

	bespoke, err := os.ReadFile(filepath.Join(repo, "packages", "bespoke", tmpl.Melange.Bespoke))
	require.NoError(t, err)
	text := string(bespoke)
	assert.Contains(t, text, "name: verity-opensearch-dashboards-config")
	assert.Contains(t, text, "sed '/^opensearch_security\\./d'")
	assert.Contains(t, text, "OPENSEARCH_HOSTS")
	assert.Contains(t, text, "opensearch.hosts")
	assert.Contains(t, text, "has_yaml_key 'opensearch\\.hosts'")
	assert.Contains(t, text, "has_yaml_key 'server\\.host'")
	assert.Contains(t, text, "SERVER_HOST")
	assert.Contains(t, text, "opensearch.username")
	assert.Contains(t, text, "exec /usr/share/opensearch-dashboards/bin/opensearch-dashboards")
	assert.False(t, strings.Contains(text, "opensearch_security.multitenancy.enabled: true"))

	values, err := os.ReadFile(filepath.Join(repo, "test", "chart-integration", "values", "opensearch-dashboards.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(values), "opensearch.hosts")
	assert.Contains(t, string(values), "opensearch.ssl.verificationMode: none")
	assert.Contains(t, string(values), "opensearchHosts: \"\"")
	assert.Contains(t, string(values), "serverHost: \"\"")
}
