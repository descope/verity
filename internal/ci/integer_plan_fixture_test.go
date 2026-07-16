package ci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func setupIntegerPlanRepo(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	writeTestFile(t, filepath.Join(root, "integer.yaml"), `
target:
  registry: ghcr.io/test-org
`)
	for _, base := range []string{"wolfi-base", "wolfi-dev", "wolfi-fips"} {
		writeTestFile(t, filepath.Join(root, "images", "_base", base+".yaml"), "# base\n")
	}
	writeTestFile(t, filepath.Join(root, "images", "node.yaml"), `
name: node
description: Node
upstream:
  package: nodejs-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}"]
  dev:
    base: wolfi-dev
    packages: ["nodejs-{{version}}", "npm"]
versions:
  "20": {}
  "22": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "curl.yaml"), `
name: curl
description: curl
upstream:
  package: curl
types:
  default:
    base: wolfi-base
    packages: ["curl"]
versions:
  latest:
    latest: true
`)
	writeTestFile(t, filepath.Join(root, "images", "caddy.yaml"), `
name: caddy
description: caddy
upstream:
  package: caddy
types:
  default:
    base: wolfi-base
    packages: ["caddy"]
  fips:
    base: wolfi-base
    fips-profile: go
    packages: ["caddy"]
    environment:
      GODEBUG: "fips140=on"
    melange:
      upstream: caddy
      env-file: fips.env
versions:
  "1": {}
  "2": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "cilium.yaml"), `
name: cilium
description: cilium
upstream:
  package: cilium-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["cilium-{{version}}"]
    melange:
      upstream: cilium-{{version}}
versions:
  "1.19": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "linkerd.yaml"), `
name: linkerd
description: linkerd
upstream:
  package: linkerd2-cli
types:
  default:
    base: wolfi-base
    packages: ["linkerd2-cli=25.12.3-r99"]
    melange:
      bespoke: linkerd2-cli-25.yaml
versions:
  "25": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "platform", "envoy.yaml"), `
name: platform/envoy
description: envoy
upstream:
  package: envoy-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["envoy-{{version}}"]
    melange:
      upstream: envoy-{{version}}
versions:
  "1.2": {}
`)
	writeTestFile(t, filepath.Join(root, "packages", "upstream.lock.json"), `
{
  "packages": {
    "caddy": {
      "file": "caddy.yaml",
      "sha256": "caddy-recipe",
      "assets": {"caddy/Caddyfile": "caddyfile"}
    },
    "cilium-1.19": {
      "file": "cilium-1.19.yaml",
      "sha256": "cilium-recipe",
      "assets": {}
    },
    "envoy-1.2": {
      "file": "envoy-1.2.yaml",
      "sha256": "envoy-recipe",
      "assets": {}
    }
  },
  "pipeline_files": {
    "build/wrapper.yaml": "wrapper",
    "go/bump.yaml": "go-bump",
    "test/ver-check.yaml": "ver-check",
    "test/unused.yaml": "unused"
  }
}
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "locked", "caddy.yaml"), `
pipeline:
  - uses: build/wrapper
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "locked", "caddy", "Caddyfile"), "test\n")
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "locked", "cilium-1.19.yaml"), `
pipeline:
  - uses: test/ver-check
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "locked", "envoy-1.2.yaml"), "pipeline: []\n")
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "linkerd2-cli-25.yaml"), "pipeline: []\n")
	writeTestFile(t, filepath.Join(root, "packages", "pipelines", "go", "bump.yaml"), "pipeline: []\n")
	writeTestFile(t, filepath.Join(root, "packages", "pipelines", "build", "wrapper.yaml"), "pipeline:\n  - uses: go/bump\n")
	writeTestFile(t, filepath.Join(root, "packages", "pipelines", "test", "ver-check.yaml"), "pipeline: []\n")
	writeTestFile(t, filepath.Join(root, "packages", "pipelines", "test", "unused.yaml"), "pipeline: []\n")
	writeTestFile(t, filepath.Join(root, "packages", "overrides", "fips.env"), "GOFIPS140=latest\n")
	return root
}
