package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/melange"
)

const intBuildNodeYAML = `
name: myapp
upstream:
  package: myapp
types:
  default:
    base: wolfi-base
    packages: [myapp]
    entrypoint: /usr/bin/myapp
versions:
  latest:
    latest: true
`

func intSetupBuildImages(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	baseDir := filepath.Join(imagesDir, "_base")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "wolfi-base.yaml"), []byte("# base\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "myapp.yaml"), []byte(intBuildNodeYAML), 0o644))
	return imagesDir
}

func intFakeApko(t *testing.T, exitCode int) {
	t.Helper()
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "apko")
	content := "#!/bin/sh\nexit " + string(rune('0'+exitCode)) + "\n"
	require.NoError(t, os.WriteFile(script, []byte(content), 0o755))
	existing := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+existing)
}

func TestIntegerBuildCommand_UnknownType(t *testing.T) {
	imagesDir := intSetupBuildImages(t)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "myapp",
		"--version", "latest",
		"--type", "jre",
		"--images-dir", imagesDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerVariantNotFound)
}

func TestIntegerBuildCommand_MissingImage(t *testing.T) {
	imagesDir := intSetupBuildImages(t)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "nonexistent",
		"--version", "latest",
		"--images-dir", imagesDir,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestIntegerRunApkoBuild_NotInPath(t *testing.T) {
	t.Setenv("PATH", "")
	err := integerRunApkoBuild(context.Background(), "config.yaml", "out.tar", "amd64", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apko not found")
}

func TestIntegerRunApkoBuild_Fails(t *testing.T) {
	intFakeApko(t, 1)
	err := integerRunApkoBuild(context.Background(), "config.yaml", "out.tar", "amd64", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apko build failed")
}

func TestIntegerRunApkoBuild_Success(t *testing.T) {
	intFakeApko(t, 0)
	err := integerRunApkoBuild(context.Background(), "config.yaml", "out.tar", "amd64", nil, nil)
	require.NoError(t, err)
}

func TestIntegerResolveLatestVersion_Success(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\nP:nodejs-24\nV:24.0.0\n\n")

	def := &intconfig.ImageDef{
		Name:     "node",
		Upstream: intconfig.Upstream{Package: "nodejs-{{version}}"},
	}

	v, err := integerResolveLatestVersion(def, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "24", v)
}

func TestIntegerResolveLatestVersion_NoVersions(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:curl\nV:8.0.0\n\n")

	def := &intconfig.ImageDef{
		Name:     "node",
		Upstream: intconfig.Upstream{Package: "nodejs-{{version}}"},
	}

	_, err := integerResolveLatestVersion(def, srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no versions found")
}

func TestIntegerBuildCommand_RejectsUndeclaredVersionWhenAutoDiscoveryDisabled(t *testing.T) {
	const yamlBody = `
name: teleport
upstream:
  package: "teleport-{{version}}"
  auto-discover: false
types:
  default:
    base: wolfi-base
    packages: ["teleport-{{version}}"]
    entrypoint: /usr/bin/teleport start
versions:
  "18.6": {}
`
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	baseDir := filepath.Join(imagesDir, "_base")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "wolfi-base.yaml"), []byte("# base\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "teleport.yaml"), []byte(yamlBody), 0o644))

	srv := intMakeAPKINDEXServer(t, "P:teleport-17\nV:17.7.8-r0\n\nP:teleport-18\nV:18.6.6-r0\n\nP:teleport-18.6\nV:18.6.6-r0\n\n")
	t.Setenv("PATH", "")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "teleport",
		"--version", "18",
		"--type", "default",
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--output", filepath.Join(dir, "image.tar"),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerVariantNotFound)
}

// intCapturingApko installs a fake `apko` on PATH that copies its config-file
// argument to capturePath and exits 0. The capture path's parent directory is
// created if needed. The fake mirrors the real apko CLI surface used by
// integerRunApkoBuild:
//
//	apko build --arch <arch> <configFile> integer:local <output>
//
// so $4 is the config file path. The caller passes capturePath in and reads
// it back after the build action returns; this helper has no return value.
func intCapturingApko(t *testing.T, capturePath string) {
	t.Helper()
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "apko")
	// Repository and keyring flags are optional, but the config is always
	// the third argument from the end.
	body := "#!/bin/sh\nset -e\nmkdir -p \"$(dirname \"" + capturePath + "\")\"\nwhile [ \"$#\" -gt 3 ]; do shift; done\ncp \"$1\" \"" + capturePath + "\"\nexit 0\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))
}

// TestIntegerBuildCommand_FloatingMajorAliasResolved is the regression for
// the PR #307 follow-up: discovery resolves floating-major streams to a
// concrete Wolfi minor stem, but the local CLI build path
// (cmd/integer_build.go) used to skip alias resolution entirely and pass
// the user-supplied --version straight into render.Config. That meant
// `verity integer build --image kyverno --version 1` rendered an apko
// config containing the unsatisfiable constraint `kyverno-1`, even though
// Wolfi only publishes `kyverno-1.17`.
//
// With the fix, both paths share discovery.ResolveStreamRenderVersion, so
// the rendered apko config substitutes the highest matching minor stem.
// The test fakes apko (no real build) and inspects the captured config.
//
// The table covers each VersionedPackagePattern shape that production
// dispatches actually exercise:
//
//   - kyverno: versioned upstream.package + versioned type packages
//     (the most common shape; cilium, crossplane, fluent-bit also
//     match this).
//   - prometheus: versioned upstream.package, declared "2" stream
//     aliases to a published minor (the originally-reported failure
//     class — "nothing provides prometheus-2.55" — generalised here so
//     the assertion stays meaningful even after dropped streams are
//     pruned from images/prometheus.yaml).
//   - erlang: unversioned upstream.package + versioned type packages —
//     forces the VersionedPackagePattern walk into types[*].packages.
//
// Each sub-case would have failed against pre-fix cmd/integer_build.go.
func TestIntegerBuildCommand_FloatingMajorAliasResolved(t *testing.T) {
	tests := []struct {
		name              string
		imageName         string
		imageYAML         string
		streamVersion     string // value passed to --version
		apkindexBody      string
		mustContain       string // aliased package constraint that must appear in the rendered apko config
		mustNotContainRaw string // unaliased literal that would crash apko publish
	}{
		{
			name:      "kyverno-shape: versioned upstream + versioned type packages",
			imageName: "kyverno",
			imageYAML: `
name: kyverno
upstream:
  package: "kyverno-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["kyverno-{{version}}"]
    entrypoint: /usr/bin/kyverno
versions:
  "1": {}
`,
			streamVersion:     "1",
			apkindexBody:      "P:kyverno-1.17\nV:1.17.5-r0\n\n",
			mustContain:       "kyverno-1.17",
			mustNotContainRaw: "- kyverno-1\n",
		},
		{
			name:      "prometheus-shape: versioned upstream, floating stream aliases to minor",
			imageName: "prometheus",
			// Mirrors images/prometheus.yaml: versioned upstream.package
			// + versioned type packages. Declared stream "2" aliases up
			// to the highest matching minor in APKINDEX (here "2.55").
			// This is exactly the shape of the originally-reported
			// production failure — the bug rendered "prometheus-2"
			// verbatim and apko publish failed with `nothing provides`.
			imageYAML: `
name: prometheus
upstream:
  package: "prometheus-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["prometheus-{{version}}"]
    entrypoint: /usr/bin/prometheus
versions:
  "2": {}
`,
			streamVersion:     "2",
			apkindexBody:      "P:prometheus-2.55\nV:2.55.4-r0\n\n",
			mustContain:       "prometheus-2.55",
			mustNotContainRaw: "- prometheus-2\n",
		},
		{
			name:      "erlang-shape: unversioned upstream + versioned type packages",
			imageName: "erlang",
			// upstream.package has no "{{version}}", so
			// VersionedPackagePattern must walk types[*].packages to find
			// the constraint shape. Aliasing fires off "erlang-{{version}}",
			// not "erlang".
			imageYAML: `
name: erlang
upstream:
  package: erlang
types:
  default:
    base: wolfi-base
    packages: ["erlang-{{version}}"]
    entrypoint: /usr/bin/erl
versions:
  "26": {}
`,
			streamVersion:     "26",
			apkindexBody:      "P:erlang\nV:27.0-r0\n\nP:erlang-26.3\nV:26.3.0.0-r0\n\n",
			mustContain:       "erlang-26.3",
			mustNotContainRaw: "- erlang-26\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			imagesDir := filepath.Join(dir, "images")
			baseDir := filepath.Join(imagesDir, "_base")
			require.NoError(t, os.MkdirAll(baseDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(baseDir, "wolfi-base.yaml"), []byte("# base\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(imagesDir, tt.imageName+".yaml"), []byte(tt.imageYAML), 0o644))

			srv := intMakeAPKINDEXServer(t, tt.apkindexBody)

			capture := filepath.Join(dir, "captured.apko.yaml")
			intCapturingApko(t, capture)

			root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
			err := root.Run(context.Background(), []string{
				"verity", "integer", "build",
				"--image", tt.imageName,
				"--version", tt.streamVersion,
				"--type", "default",
				"--images-dir", imagesDir,
				"--apkindex-url", srv.URL,
				"--output", filepath.Join(dir, "image.tar"),
			})
			require.NoError(t, err)

			rendered, err := os.ReadFile(capture)
			require.NoError(t, err, "fake apko should have captured the rendered config")
			require.NotEmpty(t, rendered)

			// The rendered apko config must reference the aliased
			// minor stem, NOT the bare floating-major literal.
			assert.Contains(t, string(rendered), tt.mustContain,
				"build path must alias declared stream %q to the highest matching minor when Wolfi only publishes the per-minor APK", tt.streamVersion)
			assert.NotContains(t, string(rendered), tt.mustNotContainRaw,
				"unaliased literal would crash apko publish — guard against regression of the cmd/integer_build.go alias gap")
		})
	}
}

func TestIntegerBuildCommand_FloatingMajorAliasBuildsAndPinsResolvedBespokePackage(t *testing.T) {
	// Given: a floating stream whose bespoke package name is versioned and whose
	// published package stem resolves to a concrete minor.
	repoRoot := t.TempDir()
	chdirIntegerMelangeTest(t, repoRoot)
	imagesDir := filepath.Join(repoRoot, "images")
	require.NoError(t, os.MkdirAll(filepath.Join(imagesDir, "_base"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "_base", "wolfi-base.yaml"), []byte("# base\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "kyverno.yaml"), []byte(`
name: kyverno
upstream:
  package: "kyverno-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["kyverno-{{version}}"]
    entrypoint: /usr/bin/kyverno
    melange:
      upstream: "kyverno-{{version}}"
versions:
  "1": {}
`), 0o644))

	var captured melange.BuildOptions
	originalBuild := integerMelangeBuild
	integerMelangeBuild = func(_ context.Context, options *melange.BuildOptions) error {
		captured = *options
		return writeIntegerMelangeArtifacts(t, options)
	}
	t.Cleanup(func() {
		integerMelangeBuild = originalBuild
	})
	srv := intMakeAPKINDEXServer(t, "P:kyverno-1.17\nV:1.17.5-r0\n\n")
	capturePath := filepath.Join(repoRoot, "captured.apko.yaml")
	intCapturingApko(t, capturePath)

	// When: the floating stream is built.
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "kyverno",
		"--version", "1",
		"--type", "default",
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--output", filepath.Join(repoRoot, "image.tar"),
	})

	// Then: recipe resolution, package output, pinning, and rendering all use
	// the same concrete minor version.
	require.NoError(t, err)
	assert.Equal(t, "kyverno-1.17", captured.Spec.Upstream)
	rendered, err := os.ReadFile(capturePath)
	require.NoError(t, err)
	assert.Contains(t, string(rendered), "kyverno-1.17=1.0-r0@local")
}

// TestIntegerBuildCommand_OfflineLeavesVersionUnchanged locks in the
// offline contract for the local CLI build path: when --apkindex-url is
// empty the build runs without fetching APKINDEX and renders the
// declared stream verbatim. Mirrors discovery's pkgs==nil behaviour so
// `verity integer build` stays usable in air-gapped or test environments.
func TestIntegerBuildCommand_OfflineLeavesVersionUnchanged(t *testing.T) {
	const yamlBody = `
name: myapp
upstream:
  package: "myapp-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["myapp-{{version}}"]
    entrypoint: /usr/bin/myapp
versions:
  "7": {}
`
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	baseDir := filepath.Join(imagesDir, "_base")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "wolfi-base.yaml"), []byte("# base\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "myapp.yaml"), []byte(yamlBody), 0o644))

	capture := filepath.Join(dir, "captured.apko.yaml")
	intCapturingApko(t, capture)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "myapp",
		"--version", "7",
		"--type", "default",
		"--images-dir", imagesDir,
		"--apkindex-url", "",
		"--output", filepath.Join(dir, "image.tar"),
	})
	require.NoError(t, err)

	rendered, err := os.ReadFile(capture)
	require.NoError(t, err)
	assert.Contains(t, string(rendered), "myapp-7",
		"offline build (--apkindex-url='') must render the declared stream verbatim")
}

// TestIntegerBuildCommand_LatestOfflineFailsFast is the regression for
// PR #311 review thread MKoO. With --apkindex-url="" the build path
// cannot consult Wolfi to determine which stream is "latest", so
// silently falling back to the highest *declared* version key (which
// is what integerResolveLatestVersionFromPkgs would do with empty
// pkgs) hides a real configuration error. The build must fail fast
// with errIntegerLatestOffline so the user knows to either pass an
// explicit --version or wire APKINDEX.
func TestIntegerBuildCommand_LatestOfflineFailsFast(t *testing.T) {
	const yamlBody = `
name: myapp
upstream:
  package: "myapp-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["myapp-{{version}}"]
    entrypoint: /usr/bin/myapp
versions:
  "7": {}
`
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	baseDir := filepath.Join(imagesDir, "_base")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "wolfi-base.yaml"), []byte("# base\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "myapp.yaml"), []byte(yamlBody), 0o644))

	// Don't install a fake apko — the failure must trip BEFORE we
	// render or call apko.
	t.Setenv("PATH", "")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "myapp",
		"--version", latestSentinel,
		"--type", "default",
		"--images-dir", imagesDir,
		"--apkindex-url", "",
		"--output", filepath.Join(dir, "image.tar"),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerLatestOffline)
}

// intCountingAPKINDEXServer returns an httptest.Server that records
// every incoming request in *count and serves a valid APKINDEX
// tar.gz for the supplied content. Mirrors intMakeAPKINDEXServer's
// shape so tests can swap one for the other to instrument fetch
// counts.
func intCountingAPKINDEXServer(t *testing.T, content string, count *int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(count, 1)
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		data := []byte(content)
		if err := tw.WriteHeader(&tar.Header{Name: "APKINDEX", Mode: 0o644, Size: int64(len(data))}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := tw.Write(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tw.Close()
		gz.Close()
		if _, err := w.Write(buf.Bytes()); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestIntegerBuildCommand_LazyAPKINDEXFetch is the regression for
// PR #311 review thread MKoM. The pre-#311 contract was that explicit
// versions did not require network. Wiring APKINDEX into the build
// path for alias resolution accidentally broke that for every
// invocation — the bot flagged it as a behavioural regression.
//
// Fix: only fetch when (a) version is `latest` (which needs APKINDEX
// to resolve), or (b) the version looks like a floating stream
// (≤ 1 dot) that might need aliasing AND the def has a versioned
// package pattern.
//
// The table covers the matrix:
//
//   - fully-pinned version + versioned pattern → NO fetch
//   - floating-major + versioned pattern → fetch (alias case)
//   - any version + unversioned (bespoke) pattern → NO fetch
func TestIntegerBuildCommand_LazyAPKINDEXFetch(t *testing.T) {
	const versionedYAML = `
name: kyverno
upstream:
  package: "kyverno-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["kyverno-{{version}}"]
    entrypoint: /usr/bin/kyverno
versions:
  "1": {}
  "1.17.5": {}
`
	const unversionedYAML = `
name: popeye
upstream:
  package: popeye
types:
  default:
    base: wolfi-base
    packages: [popeye]
    entrypoint: /usr/bin/popeye
versions:
  "0.22.1": {}
`

	tests := []struct {
		name        string
		imageName   string
		yamlBody    string
		version     string
		wantFetches int64
	}{
		{
			name:        "fully-pinned version on versioned-pattern image skips fetch",
			imageName:   "kyverno",
			yamlBody:    versionedYAML,
			version:     "1.17.5",
			wantFetches: 0,
		},
		{
			name:        "floating-major on versioned-pattern image triggers fetch",
			imageName:   "kyverno",
			yamlBody:    versionedYAML,
			version:     "1",
			wantFetches: 1,
		},
		{
			name:        "explicit version on bespoke (unversioned) image skips fetch",
			imageName:   "popeye",
			yamlBody:    unversionedYAML,
			version:     "0.22.1",
			wantFetches: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			imagesDir := filepath.Join(dir, "images")
			baseDir := filepath.Join(imagesDir, "_base")
			require.NoError(t, os.MkdirAll(baseDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(baseDir, "wolfi-base.yaml"), []byte("# base\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(imagesDir, tt.imageName+".yaml"), []byte(tt.yamlBody), 0o644))

			var fetches int64
			srv := intCountingAPKINDEXServer(t,
				"P:kyverno-1.17\nV:1.17.5-r0\n\nP:kyverno-1.17.5\nV:1.17.5-r0\n\nP:popeye\nV:0.22.1-r0\n\n",
				&fetches)

			capture := filepath.Join(dir, "captured.apko.yaml")
			intCapturingApko(t, capture)

			root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
			err := root.Run(context.Background(), []string{
				"verity", "integer", "build",
				"--image", tt.imageName,
				"--version", tt.version,
				"--type", "default",
				"--images-dir", imagesDir,
				"--apkindex-url", srv.URL,
				"--output", filepath.Join(dir, "image.tar"),
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantFetches, atomic.LoadInt64(&fetches),
				"build path must only fetch APKINDEX when alias resolution actually needs it")
		})
	}
}

func intFakeTrivy(t *testing.T, exitCode int) {
	t.Helper()
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "trivy")
	content := "#!/bin/sh\nif [ \"$2\" = \"--download-db-only\" ]; then exit 0; fi\nexit " + string(rune('0'+exitCode)) + "\n"
	require.NoError(t, os.WriteFile(script, []byte(content), 0o755))
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))
}

func TestIntegerTrivyGate_Clean(t *testing.T) {
	intFakeTrivy(t, 0)
	require.NoError(t, integerTrivyGate(context.Background(), "image.tar", "HIGH,CRITICAL"))
}

func TestIntegerTrivyGate_Findings(t *testing.T) {
	intFakeTrivy(t, 1)
	err := integerTrivyGate(context.Background(), "image.tar", "HIGH,CRITICAL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to publish")
}
