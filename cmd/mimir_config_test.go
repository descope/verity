package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Mimir_uses_stream_scoped_bespoke_packages(t *testing.T) {
	// Given: the checked-in Grafana Mimir image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "mimir.yaml"))
	require.NoError(t, err)

	// When: the default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: both supported streams select their matching local rebuild.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "grafana-mimir-{{version}}.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"grafana-mimir-{{version}}"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/mimir", tmpl.Entrypoint)
	require.Contains(t, def.Versions, "3.0")
	require.Contains(t, def.Versions, "3.1")
	require.True(t, def.Versions["3.1"].Latest)
}

func Test_Mimir_recipes_pin_fixed_sources(t *testing.T) {
	tests := []struct {
		name     string
		stream   string
		version  string
		epoch    string
		commit   string
		checksum string
		branch   string
		baseline string
	}{
		{
			name:     "3.0 stream",
			stream:   "3.0",
			version:  "3.0.7",
			epoch:    "3",
			commit:   "c8a2454ab4fb93789a829890a63bc89107707114",
			checksum: "9e108dbd5aaec06db4b1c7d7179016209926a854419bdc4b500d42e43e14371e",
			branch:   "release-3.0",
			baseline: "b137f48f69fd0d54416849cf09201ab5ff519562",
		},
		{
			name:     "3.1 stream",
			stream:   "3.1",
			version:  "3.1.2",
			epoch:    "2",
			commit:   "e23940eb3a28c3e3f34968aa57d6a189370aa0f7",
			checksum: "f3a556c4fb50b00e88d5093570e209ebf54bd5b9d6c2d07606c8baa68f25e9bf",
			branch:   "release-3.1",
			baseline: "bfb9155301a240694c98ec1f1f3e1a6fee92031f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: the bespoke recipe selected for this Mimir stream.
			recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "grafana-mimir-"+tt.stream+".yaml"))
			require.NoError(t, err)

			// When: its package, source, dependency, test, and license contracts are inspected.
			text := string(recipe)

			// Then: the fixed release is reproducible and carries the strict runtime evidence hooks.
			require.Contains(t, text, "name: grafana-mimir-"+tt.stream)
			require.Contains(t, text, "version: \""+tt.version+"\"")
			require.Contains(t, text, "epoch: "+tt.epoch)
			require.Contains(t, text, "license: AGPL-3.0-or-later")
			require.Contains(t, text, "Adapted from wolfi-dev/os@"+tt.baseline)
			require.Contains(t, text, tt.commit+".tar.gz")
			require.Contains(t, text, "expected-sha256: "+tt.checksum)
			require.Contains(t, text, "      - git\n")
			require.Contains(t, text, "golang.org/x/net@v0.57.0")
			require.Contains(t, text, "golang.org/x/text@v0.40.0")
			require.Contains(t, text, "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp@v1.44.0")
			require.Contains(t, text, "github.com/grafana/mimir/pkg/util/version.Branch="+tt.branch)
			require.Contains(t, text, "github.com/grafana/mimir/pkg/util/version.Revision="+tt.commit)
			require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
			require.Contains(t, text, "apk info --who-owns /usr/bin/grafana-mimir")
			require.Contains(t, text, "go mod edit \\")
			require.Contains(t, text, "rm -rf vendor")
			require.NotContains(t, text, "- uses: go/bump")
			require.Less(t, strings.Index(text, "rm -f .gitconfig"), strings.Index(text, "go mod edit \\"))
		})
	}
}

func Test_Mimir_resolves_fixed_local_packages(t *testing.T) {
	tests := []struct {
		name        string
		stream      string
		fullVersion string
	}{
		{name: "3.0 stream", stream: "3.0", fullVersion: "3.0.7-r3"},
		{name: "3.1 stream", stream: "3.1", fullVersion: "3.1.2-r2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: the default image template and the package built for this stream.
			def, err := intconfig.LoadImage(filepath.Join("..", "images", "mimir.yaml"))
			require.NoError(t, err)
			tmpl := def.Types["default"]

			// When: the local Melange artifact is pinned for image assembly.
			err = pinLocalPackageVersions(&tmpl, tt.stream, []apkindex.Package{{
				Name:    "grafana-mimir-" + tt.stream,
				Version: tt.fullVersion,
			}})

			// Then: apko can only select the approved fixed local revision.
			require.NoError(t, err)
			require.Equal(t, []string{"grafana-mimir-" + tt.stream + "=" + tt.fullVersion + "@local"}, tmpl.Packages)
		})
	}
}
