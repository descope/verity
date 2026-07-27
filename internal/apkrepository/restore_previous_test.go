package apkrepository

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
	"github.com/verity-org/verity/internal/ci/sitepublication"
)

const restoreSourceSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type restorePublicationRunner struct{}

func (restorePublicationRunner) Run(context.Context, publication.Command) (publication.CommandResult, error) {
	return publication.CommandResult{}, nil
}

func TestRestorePrevious_restores_only_exact_attested_Build_Site_artifact(t *testing.T) {
	// Given an exact successful Build Site run and digest-bound artifact.
	fixture := newRestoreFixture(t)
	output := filepath.Join(t.TempDir(), "restored")
	runner := fixture.runner(t, nil)
	var stdout bytes.Buffer

	// When the protected restore is authorized.
	err := RestorePrevious(context.Background(), fixture.options(output, &stdout, runner))

	// Then the complete site is restored and the attestation precedes extraction.
	require.NoError(t, err)
	assert.Equal(t, "site", string(readTestFile(t, filepath.Join(output, "index.html"))))
	assert.Equal(t, "apk", string(readTestFile(t, filepath.Join(output, "apk", "x86_64", "demo.apk"))))
	repacked := filepath.Join(t.TempDir(), "repacked.tar")
	_, err = sitepublication.PackSite(output, repacked)
	require.NoError(t, err)
	assert.Equal(t, fixture.tarBytes, readTestFile(t, repacked))
	assert.Contains(t, stdout.String(), `"restored":true`)
	assert.Equal(t, "gh attestation verify", runner.calls[3].name+" "+strings.Join(runner.calls[3].args[:2], " "))
}

func TestRestorePrevious_rejects_wrong_workflow_run_standalone_and_legacy_artifacts(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*restoreFixture)
		wantErr error
	}{
		{name: "wrong workflow", mutate: func(f *restoreFixture) { f.workflow = ".github/workflows/apk-repository.yaml" }, wantErr: errWrongPagesWorkflow},
		{name: "wrong run", mutate: func(f *restoreFixture) { f.returnedRunID = 61 }, wantErr: errWrongPagesRun},
		{name: "wrong attempt", mutate: func(f *restoreFixture) { f.returnedAttempt = 3 }, wantErr: errWrongPagesRun},
		{name: "standalone APK artifact", mutate: func(f *restoreFixture) { f.artifactName = "apk-repository" }, wantErr: errPagesArtifactNotFound},
		{name: "legacy artifact digest", mutate: func(f *restoreFixture) { f.zipDigest = "" }, wantErr: errLegacyPagesArtifact},
		{name: "legacy site payload", mutate: func(f *restoreFixture) {
			f.tarBytes = []byte("legacy")
			sum := sha256.Sum256(f.tarBytes)
			f.artifactDigest = "sha256:" + hex.EncodeToString(sum[:])
		}, wantErr: sitepublication.ErrInvalidArchive},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one untrusted prior-artifact mutation.
			fixture := newRestoreFixture(t)
			test.mutate(fixture)
			if test.name == "legacy site payload" {
				fixture.rebuildZip(t)
			}
			output := filepath.Join(t.TempDir(), "restored")

			// When restore evaluates the exact run/artifact contract.
			err := RestorePrevious(context.Background(), fixture.options(output, nil, fixture.runner(t, nil)))

			// Then no prior bytes are committed.
			require.ErrorIs(t, err, test.wantErr)
			assert.NoDirExists(t, output)
		})
	}
}

func TestRestorePrevious_requires_authorization_and_rejects_tampering(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RestorePreviousOptions)
	}{
		{name: "authorization missing", mutate: func(options *RestorePreviousOptions) { options.AuthorizeRestore = false }},
		{name: "artifact digest tampered", mutate: func(options *RestorePreviousOptions) {
			options.ExpectedArtifactDigest = "sha256:" + strings.Repeat("f", 64)
		}},
		{name: "manifest digest tampered", mutate: func(options *RestorePreviousOptions) {
			options.ExpectedManifestDigest = "sha256:" + strings.Repeat("e", 64)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an otherwise exact restore request.
			fixture := newRestoreFixture(t)
			output := filepath.Join(t.TempDir(), "restored")
			options := fixture.options(output, nil, fixture.runner(t, nil))
			test.mutate(options)

			// When restore is attempted.
			err := RestorePrevious(context.Background(), options)

			// Then authorization or digest validation fails closed.
			require.Error(t, err)
			assert.NoDirExists(t, output)
		})
	}
}

type restoreFixture struct {
	runID, returnedRunID           uint64
	attempt, returnedAttempt       uint64
	workflow, artifactName         string
	manifestDigest, artifactDigest string
	tarBytes, zipBytes             []byte
	zipDigest                      string
}

func newRestoreFixture(t *testing.T) *restoreFixture {
	t.Helper()
	root := t.TempDir()
	manifest := publication.Manifest{
		SchemaVersion: publication.SchemaVersion, SourceSHA: restoreSourceSHA,
		RunID: 60, RunAttempt: 2, BatchID: "60-2", Mode: publication.ModeBootstrap,
		Components: []publication.Component{{
			Name: "catalog", Kind: publication.ComponentKindGeneric, ArtifactName: "catalog-60-2",
			ArtifactDigest: publication.Digest("sha256:" + strings.Repeat("1", 64)), Workflow: ".github/workflows/build-site-catalog.yaml",
			Event: publication.EventWorkflowCall, Result: publication.ResultSuccess,
		}, {
			Name: "integer-x86", Kind: publication.ComponentKindAPK, Architecture: publication.ArchitectureX8664,
			ArtifactName: "integer-60-2-x86", ArtifactDigest: publication.Digest("sha256:" + strings.Repeat("3", 64)),
			Workflow: ".github/workflows/integer-build-image.yaml", Event: publication.EventWorkflowCall, Result: publication.ResultSuccess,
		}},
		SignerDigest: publication.Digest("sha256:" + strings.Repeat("2", 64)),
		APKOperations: []publication.APKOperation{{
			Action: publication.APKUpsert, Architecture: publication.ArchitectureX8664, PackageName: "demo",
			ArtifactName: "integer-60-2-x86", ArtifactDigest: publication.Digest("sha256:" + strings.Repeat("3", 64)),
		}},
	}
	plan, err := sitepublication.CreatePlan(context.Background(), &sitepublication.PlanRequest{
		Manifest:         manifest,
		ExpectedIdentity: publication.ProducerIdentity{SourceSHA: manifest.SourceSHA, RunID: manifest.RunID, RunAttempt: manifest.RunAttempt, BatchID: manifest.BatchID},
		ExpectedMode:     manifest.Mode, ExpectedComponents: manifest.Components, PublicationSHA: manifest.SourceSHA,
		SignerLock:              signerlock.Lock{Image: signerlock.SignerImageRepository, Digest: string(manifest.SignerDigest), Workflow: signerlock.TrustedWorkflowIdentity, SourceSHA: strings.Repeat("c", 40), Runnable: true},
		ExpectedSignerSourceSHA: strings.Repeat("c", 40), AuthorizeBootstrap: true, Runner: restorePublicationRunner{},
	})
	require.NoError(t, err)
	overlay := filepath.Join(root, "overlay")
	writeTestFile(t, filepath.Join(overlay, "index.html"), "site")
	apk := filepath.Join(root, "apk")
	writeTestFile(t, filepath.Join(apk, "x86_64", "demo.apk"), "apk")
	writeTestFile(t, filepath.Join(apk, "aarch64", "demo.apk"), "apk-arm")
	site := filepath.Join(root, "site")
	_, err = sitepublication.AssembleSite(context.Background(), &sitepublication.AssembleRequest{
		Plan: plan, Manifest: manifest, SignedAPKDir: apk, OutputDir: site,
		Overlays: []sitepublication.Overlay{{Name: "site", SourceDir: overlay}},
	})
	require.NoError(t, err)
	tarPath := filepath.Join(root, "artifact.tar")
	artifactDigest, err := sitepublication.PackSite(site, tarPath)
	require.NoError(t, err)
	manifestDigest, err := publication.DigestManifest(&manifest)
	require.NoError(t, err)
	fixture := &restoreFixture{
		runID: 60, returnedRunID: 60, attempt: 2, returnedAttempt: 2,
		workflow: sitepublication.PublishWorkflow, artifactName: sitepublication.PagesArtifactName,
		manifestDigest: string(manifestDigest), artifactDigest: string(artifactDigest), tarBytes: readTestFile(t, tarPath),
	}
	fixture.rebuildZip(t)
	return fixture
}

func (fixture *restoreFixture) rebuildZip(t *testing.T) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("artifact.tar")
	require.NoError(t, err)
	_, err = entry.Write(fixture.tarBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	fixture.zipBytes = buffer.Bytes()
	sum := sha256.Sum256(fixture.zipBytes)
	fixture.zipDigest = "sha256:" + hex.EncodeToString(sum[:])
}

func (fixture *restoreFixture) options(output string, stdout *bytes.Buffer, runner *fakeCommandRunner) *RestorePreviousOptions {
	return &RestorePreviousOptions{
		OutputDir: output, Repository: "verity-org/verity", RunID: fixture.runID, RunAttempt: fixture.attempt,
		ExpectedSourceSHA: restoreSourceSHA, ExpectedArtifactDigest: fixture.artifactDigest,
		ExpectedManifestDigest: fixture.manifestDigest, AuthorizeRestore: true, Stdout: stdout, runner: runner,
	}
}

func (fixture *restoreFixture) runner(t *testing.T, attestationError error) *fakeCommandRunner {
	t.Helper()
	runner := &fakeCommandRunner{}
	runner.run = func(request command) (commandResult, error) {
		joined := strings.Join(request.args, " ")
		switch {
		case strings.Contains(joined, fmt.Sprintf("actions/runs/%d/artifacts", fixture.runID)):
			body := fmt.Sprintf(`{"artifacts":[{"id":6,"name":%q,"expired":false,"digest":%q,"workflow_run":{"id":%d,"head_branch":"main","head_sha":%q}}]}`, fixture.artifactName, fixture.zipDigest, fixture.runID, restoreSourceSHA)
			return commandResult{stdout: []byte(body)}, nil
		case strings.Contains(joined, fmt.Sprintf("actions/runs/%d", fixture.runID)):
			body := fmt.Sprintf(`{"id":%d,"run_attempt":%d,"name":%q,"path":%q,"head_branch":"main","head_sha":%q,"conclusion":"success"}`, fixture.returnedRunID, fixture.returnedAttempt, sitepublication.PublishWorkflowName, fixture.workflow, restoreSourceSHA)
			return commandResult{stdout: []byte(body)}, nil
		case strings.Contains(joined, "actions/artifacts/6/zip"):
			_, err := request.stdout.Write(fixture.zipBytes)
			return commandResult{}, err
		case strings.HasPrefix(joined, "attestation verify "):
			return commandResult{}, attestationError
		default:
			return commandResult{}, assert.AnError
		}
	}
	return runner
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
