package sitepublication

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestAssembleSite_overlays_deterministically_and_preserves_unlisted_bytes(t *testing.T) {
	// Given previous site/catalog/chart/APK bytes and two declared overlays.
	plan, manifest := validPlanAndManifest(t)
	root := t.TempDir()
	base := filepath.Join(root, "base")
	writeSiteFile(t, base, "index.html", "old-site")
	writeSiteFile(t, base, "catalog/keep.json", "keep-catalog")
	writeSiteFile(t, base, "catalog/update.json", "old-catalog")
	writeSiteFile(t, base, "charts/keep.tgz", "keep-chart")
	writeSiteFile(t, base, "apk/x86_64/keep.apk", "keep-x86")
	writeSiteFile(t, base, "apk/aarch64/keep.apk", "keep-arm")
	sealSiteFixture(t, base, validPlanRequest(t).PreviousManifest)
	signedAPK := filepath.Join(root, "signed-apk")
	writeSiteFile(t, signedAPK, "x86_64/keep.apk", "keep-x86")
	writeSiteFile(t, signedAPK, "x86_64/new.apk", "new-x86")
	writeSiteFile(t, signedAPK, "aarch64/keep.apk", "keep-arm")
	siteOverlay := filepath.Join(root, "site-overlay")
	writeSiteFile(t, siteOverlay, "index.html", "new-site")
	writeSiteFile(t, siteOverlay, "catalog/update.json", "new-catalog")
	chartOverlay := filepath.Join(root, "chart-overlay")
	writeSiteFile(t, chartOverlay, "charts/new.tgz", "new-chart")
	overlays := []Overlay{
		{Name: "site", SourceDir: siteOverlay},
		{Name: "charts", SourceDir: chartOverlay},
	}

	// When the same inputs are assembled in opposite overlay order.
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	_, err := AssembleSite(context.Background(), &AssembleRequest{
		Plan: plan, Manifest: manifest, BaseDir: base, SignedAPKDir: signedAPK,
		OutputDir: first, Overlays: overlays,
	})
	require.NoError(t, err)
	_, err = AssembleSite(context.Background(), &AssembleRequest{
		Plan: plan, Manifest: manifest, BaseDir: base, SignedAPKDir: signedAPK,
		OutputDir: second, Overlays: []Overlay{overlays[1], overlays[0]},
	})
	require.NoError(t, err)
	firstArchive := filepath.Join(root, "first.tar")
	secondArchive := filepath.Join(root, "second.tar")
	firstDigest, err := PackSite(first, firstArchive)
	require.NoError(t, err)
	secondDigest, err := PackSite(second, secondArchive)
	require.NoError(t, err)

	// Then unlisted bytes survive and archive bytes are order-independent.
	assert.Equal(t, "keep-catalog", readSiteFile(t, first, "catalog/keep.json"))
	assert.Equal(t, "keep-chart", readSiteFile(t, first, "charts/keep.tgz"))
	assert.Equal(t, "keep-x86", readSiteFile(t, first, "apk/x86_64/keep.apk"))
	assert.Equal(t, "new-site", readSiteFile(t, first, "index.html"))
	assert.Equal(t, "new-catalog", readSiteFile(t, first, "catalog/update.json"))
	assert.Equal(t, firstDigest, secondDigest)
	assert.True(t, bytes.Equal(readFile(t, firstArchive), readFile(t, secondArchive)))
}

func TestAssembleSite_rejects_conflicting_overlay_mutations(t *testing.T) {
	// Given two producer overlays targeting the same site path.
	plan, manifest := validPlanAndManifest(t)
	root := t.TempDir()
	base := filepath.Join(root, "base")
	writeSiteFile(t, base, "index.html", "old")
	sealSiteFixture(t, base, validPlanRequest(t).PreviousManifest)
	signedAPK := filepath.Join(root, "apk")
	writeSiteFile(t, signedAPK, "x86_64/demo.apk", "x86")
	writeSiteFile(t, signedAPK, "aarch64/demo.apk", "arm")
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	writeSiteFile(t, left, "catalog/shared.json", "left")
	writeSiteFile(t, right, "catalog/shared.json", "right")

	// When assembly encounters the undeclared ordering dependency.
	_, err := AssembleSite(context.Background(), &AssembleRequest{
		Plan: plan, Manifest: manifest, BaseDir: base, SignedAPKDir: signedAPK,
		OutputDir: filepath.Join(root, "output"),
		Overlays:  []Overlay{{Name: "left", SourceDir: left}, {Name: "right", SourceDir: right}},
	})

	// Then it rejects the ambiguous mutation instead of choosing an order.
	require.ErrorIs(t, err, ErrOverlayConflict)
}

func TestAssembleSite_rejects_base_site_outside_manifest_CAS(t *testing.T) {
	// Given a plan for one prior manifest and a coherent site from another run.
	plan, manifest := validPlanAndManifest(t)
	root := t.TempDir()
	base := filepath.Join(root, "base")
	writeSiteFile(t, base, "index.html", "stale")
	stale := testManifest(publication.ModeBootstrap, testPreviousSHA, 40, 1)
	sealSiteFixture(t, base, &stale)
	signedAPK := filepath.Join(root, "apk")
	writeSiteFile(t, signedAPK, "x86_64/demo.apk", "x86")
	writeSiteFile(t, signedAPK, "aarch64/demo.apk", "arm")

	// When assembly tries to combine the exact plan with the stale site.
	_, err := AssembleSite(context.Background(), &AssembleRequest{
		Plan: plan, Manifest: manifest, BaseDir: base, SignedAPKDir: signedAPK,
		OutputDir: filepath.Join(root, "output"),
	})

	// Then the same CAS boundary rejects the mixed prior state.
	require.ErrorIs(t, err, publication.ErrCASMismatch)
}

func TestAssembleSite_full_snapshot_replaces_APK_and_preserves_site_bytes(t *testing.T) {
	// Given a snapshot plan, prior site, and a complete replacement APK tree.
	request := validPlanRequest(t)
	request.Manifest.Mode = publication.ModeSnapshot
	request.ExpectedMode = publication.ModeSnapshot
	plan, err := CreatePlan(context.Background(), request)
	require.NoError(t, err)
	root := t.TempDir()
	base := filepath.Join(root, "base")
	writeSiteFile(t, base, "index.html", "preserve-site")
	writeSiteFile(t, base, "apk/x86_64/old.apk", "old")
	writeSiteFile(t, base, "apk/aarch64/old.apk", "old")
	sealSiteFixture(t, base, request.PreviousManifest)
	signedAPK := filepath.Join(root, "signed")
	writeSiteFile(t, signedAPK, "x86_64/new.apk", "new-x86")
	writeSiteFile(t, signedAPK, "aarch64/new.apk", "new-arm")
	output := filepath.Join(root, "output")

	// When the full snapshot is assembled.
	_, err = AssembleSite(context.Background(), &AssembleRequest{
		Plan: plan, Manifest: request.Manifest, BaseDir: base, SignedAPKDir: signedAPK, OutputDir: output,
	})

	// Then only the APK subtree is replaced.
	require.NoError(t, err)
	assert.Equal(t, "preserve-site", readSiteFile(t, output, "index.html"))
	assert.NoFileExists(t, filepath.Join(output, "apk", "x86_64", "old.apk"))
	assert.Equal(t, "new-x86", readSiteFile(t, output, "apk/x86_64/new.apk"))
}

func validPlanAndManifest(t *testing.T) (PublicationPlan, publication.Manifest) {
	t.Helper()
	request := validPlanRequest(t)
	plan, err := CreatePlan(context.Background(), request)
	require.NoError(t, err)
	return plan, request.Manifest
}

func writeSiteFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readSiteFile(t *testing.T, root, relative string) string {
	t.Helper()
	return string(readFile(t, filepath.Join(root, filepath.FromSlash(relative))))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func sealSiteFixture(t *testing.T, root string, manifest *publication.Manifest) {
	t.Helper()
	manifestBytes, err := publication.MarshalCanonical(manifest)
	require.NoError(t, err)
	require.NoError(t, writeSiteBytes(root, PublicationManifestPath, manifestBytes, 0o644))
	files, err := buildSiteFileManifest(root)
	require.NoError(t, err)
	fileBytes, err := marshalSiteFileManifest(&files)
	require.NoError(t, err)
	require.NoError(t, writeSiteBytes(root, SiteFileManifestPath, fileBytes, 0o644))
}
