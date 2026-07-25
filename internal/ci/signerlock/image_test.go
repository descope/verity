package signerlock

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/render"
)

func TestSignerImage_definitionRendersMinimalNonDevRuntime(t *testing.T) {
	// Given the Integer signer image definition and its existing non-dev base.
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", ".."))
	imagePath := filepath.Join(repoRoot, "images", "apk-repository-signer.yaml")

	// When the definition is loaded and rendered for apko.
	def, err := intconfig.LoadImage(imagePath)
	require.NoError(t, err)
	require.NoError(t, intconfig.Validate(def))
	tmpl, ok := def.Types["default"]
	require.True(t, ok)
	rendered, err := render.Config(&tmpl, "", "_base")
	require.NoError(t, err)
	t.Logf("rendered apko config:\n%s", rendered)

	// Then it contains the signer runtime on the non-dev base and runs through its binary.
	assert.Equal(t, "apk-repository-signer", def.Name)
	assert.Equal(t, "apk-repository-signer", def.Upstream.Package)
	assert.Equal(t, "wolfi-base", tmpl.Base)
	assert.Equal(t, []string{"apk-repository-signer", "melange"}, tmpl.Packages)
	assert.Equal(t, "/usr/bin/apk-repository-signer", tmpl.Entrypoint)
	assert.Empty(t, tmpl.Cmd)
	assert.Empty(t, tmpl.WorkDir)
	assert.Empty(t, tmpl.Environment)
	assert.Equal(t, []intconfig.PathDef{{
		Path: "/run/verity-signing", Type: "directory", UID: 0, GID: 0, Permissions: "0o700",
	}}, tmpl.Paths)
	baseData, err := os.ReadFile(filepath.Join(repoRoot, "images", "_base", "wolfi-base.yaml"))
	require.NoError(t, err)
	var base struct {
		Accounts struct {
			RunAs int `yaml:"run-as"`
		} `yaml:"accounts"`
	}
	require.NoError(t, yaml.Unmarshal(baseData, &base))
	assert.Equal(t, 65532, base.Accounts.RunAs)

	var config map[string]any
	require.NoError(t, yaml.Unmarshal(rendered, &config))
	assert.Equal(t, "_base/wolfi-base.yaml", config["include"])
	contents, ok := config["contents"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"apk-repository-signer", "melange"}, contents["packages"])
	entrypoint, ok := config["entrypoint"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/usr/bin/apk-repository-signer", entrypoint["command"])
}
