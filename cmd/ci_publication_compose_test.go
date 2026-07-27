package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	ci "github.com/verity-org/verity/internal/ci"
	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
)

func TestCIPublicationComposeCommand_writes_exact_canonical_outputs(t *testing.T) {
	// Given exact same-run Integer and chart producer manifests.
	repository := t.TempDir()
	runGit(t, repository, "init", "-q")
	runGit(t, repository, "config", "user.email", "ci@example.invalid")
	runGit(t, repository, "config", "user.name", "CI Test")
	sourceSHA := commitFile(t, repository, "source")
	root := t.TempDir()
	integerPath := filepath.Join(root, "integer-manifest.json")
	chartPath := filepath.Join(root, "producer-manifest.json")
	publicationPath := filepath.Join(root, "publication.json")
	componentsPath := filepath.Join(root, "components.json")
	githubOutputPath := filepath.Join(root, "github-output")
	signerLockPath := filepath.Join(root, "apk-signer.lock.json")
	signingKeyStatePath := filepath.Join(root, "apk-signing-key-state.json")
	publicKeyPath := filepath.Join(repository, "keys", "apk", "verity.rsa.pub")
	require.NoError(t, os.MkdirAll(filepath.Dir(publicKeyPath), 0o755))
	publicKey, err := os.ReadFile(filepath.Join("..", "keys", "apk", "verity.rsa.pub"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(publicKeyPath, publicKey, 0o644))
	keyState := []byte(`{"schema_version":1,"epoch":1,"public_key_path":"keys/apk/verity.rsa.pub","active_fingerprint":"416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7","trusted_fingerprints":["416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7"],"revoked_fingerprints":["90f7940b20391f49b417b9b3be49f01ee88b975313860b6e1a77bbf7b109c6d2"]}`)
	require.NoError(t, os.WriteFile(signingKeyStatePath, keyState, 0o600))
	integer := commandIntegerManifest(sourceSHA)
	integerData, err := ci.MarshalIntegerBatchManifest(&integer)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(integerPath, integerData, 0o600))
	chartData := fmt.Appendf(nil, `{"version":1,"repository":"verity-org/verity","run_id":42,"run_attempt":3,"source_sha":%q,"publication_id":"build-site-42-3","artifact_name":"chart-publication-build-site-42-3"}`+"\n", sourceSHA)
	require.NoError(t, os.WriteFile(chartPath, chartData, 0o600))
	signerLockData, err := json.Marshal(signerlock.Lock{
		Image: signerlock.SignerImageRepository, Digest: commandDigest("9"),
		Workflow: signerlock.TrustedWorkflowIdentity, SourceSHA: sourceSHA, Runnable: true,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(signerLockPath, signerLockData, 0o600))

	// When the public compose command materializes both outputs.
	args := []string{
		"verity", "ci", "publication", "compose",
		"--source-sha", sourceSHA, "--run-id", "42", "--run-attempt", "3", "--batch-id", "42-3",
		"--mode", "bootstrap", "--signer-lock", signerLockPath, "--publication-sha", sourceSHA,
		"--signing-key-state", signingKeyStatePath,
		"--producer-manifest", "integer=integer-manifest-build-site-42-3=" + commandDigest("1") + "=" + integerPath,
		"--producer-manifest", "charts=chart-publication-build-site-42-3=" + commandDigest("2") + "=" + chartPath,
		"--publication-output", publicationPath, "--components-output", componentsPath,
		"--github-output", githubOutputPath,
		"--authorize-bootstrap", "--repo-dir", repository,
	}
	command := &cli.Command{Commands: []*cli.Command{CICommand}}
	err = command.Run(context.Background(), args)

	// Then both explicit files contain canonical typed records.
	require.NoError(t, err)
	publicationData, err := os.ReadFile(publicationPath)
	require.NoError(t, err)
	manifest, err := publication.ParseCanonical(publicationData)
	require.NoError(t, err)
	require.Equal(t, publication.Digest(commandDigest("9")), manifest.SignerDigest)
	require.Equal(t, uint64(1), manifest.SigningKeyEpoch)
	require.Equal(t, "416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7", manifest.ActiveSigningKeyFingerprint)
	require.Equal(t, []string{"416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7"}, manifest.TrustedSigningKeyFingerprints)
	require.Equal(t, []string{"90f7940b20391f49b417b9b3be49f01ee88b975313860b6e1a77bbf7b109c6d2"}, manifest.RevokedSigningKeyFingerprints)
	componentsData, err := os.ReadFile(componentsPath)
	require.NoError(t, err)
	components, err := publication.ParseComponentsCanonical(componentsData)
	require.NoError(t, err)
	require.Equal(t, manifest.Components, components)
	githubOutput, err := os.ReadFile(githubOutputPath)
	require.NoError(t, err)
	require.Equal(t, "signer_digest="+commandDigest("9")+"\nsigner_source_sha="+sourceSHA+"\n", string(githubOutput))
}

func TestResolveSigner_acceptsRawDigest(t *testing.T) {
	// Given a raw signer digest without a lock path.
	digest := publication.Digest(commandDigest("9"))

	// When the signer input is resolved.
	got, err := resolveSigner(string(digest), "")

	// Then the digest is preserved and no source SHA is implied.
	require.NoError(t, err)
	require.Equal(t, resolvedSigner{Digest: digest}, got)
}

func TestResolveSigner_rejectsBothRawDigestAndLock(t *testing.T) {
	// Given both mutually exclusive signer sources.

	// When the signer input is resolved.
	_, err := resolveSigner(commandDigest("9"), "signer-lock.json")

	// Then the command rejects ambiguous signer provenance before reading a file.
	require.ErrorIs(t, err, errInvalidPublicationArguments)
}

func TestResolveSigner_rejectsBootstrapLock(t *testing.T) {
	// Given the checked-in bootstrap lock state represented in a temporary file.
	path := filepath.Join(t.TempDir(), "apk-signer.lock.json")
	data, err := json.Marshal(signerlock.Lock{
		Image: signerlock.SignerImageRepository, Digest: commandDigest("9"),
		Workflow: signerlock.TrustedWorkflowIdentity, SourceSHA: strings.Repeat("a", 40),
		Bootstrap: true, Runnable: false,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	// When the signer input is resolved.
	_, err = resolveSigner("", path)

	// Then bootstrap remains fail-closed at the typed lock boundary.
	require.ErrorIs(t, err, signerlock.ErrBootstrap)
}

func TestResolveSigner_exposesTypedLockCoordinates(t *testing.T) {
	// Given a runnable lock with a pinned digest and source revision.
	path := filepath.Join(t.TempDir(), "apk-signer.lock.json")
	sourceSHA := strings.Repeat("b", 40)
	data, err := json.Marshal(signerlock.Lock{
		Image: signerlock.SignerImageRepository, Digest: commandDigest("8"),
		Workflow: signerlock.TrustedWorkflowIdentity, SourceSHA: sourceSHA, Runnable: true,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	// When the signer input is resolved.
	got, err := resolveSigner("", path)

	// Then both lock coordinates are available as typed values.
	require.NoError(t, err)
	require.Equal(t, publication.Digest(commandDigest("8")), got.Digest)
	require.Equal(t, publication.SourceSHA(sourceSHA), got.SourceSHA)
}

func commandIntegerManifest(sourceSHA string) ci.IntegerBatchManifest {
	artifact := ci.IntegerArtifactRef{PublicationID: "build-site-42-3", Name: "apk-repository-build-site-42-3-1", Digest: commandDigest("3")}
	file := ci.IntegerPackageFile{Architecture: ci.IntegerArchitectureX8664, Name: "demo", SHA256: commandDigest("4"), Path: "x86_64/demo.apk"}
	return ci.IntegerBatchManifest{
		SchemaVersion: ci.IntegerBatchSchemaVersion, SourceSHA: sourceSHA,
		RunID: 42, RunAttempt: 3, PublicationID: "build-site-42-3", BatchID: "42-3",
		Mode: ci.IntegerBatchModeSnapshot, Event: ci.IntegerBatchEventWorkflowCall,
		Shards: []ci.IntegerShardManifest{{
			SchemaVersion: ci.IntegerBatchSchemaVersion, SourceSHA: sourceSHA,
			RunID: 42, RunAttempt: 3, PublicationID: "build-site-42-3", BatchID: "42-3",
			Mode: ci.IntegerBatchModeSnapshot, Event: ci.IntegerBatchEventWorkflowCall,
			Shard: "1", Artifact: artifact, Packages: []ci.IntegerPackageFile{file},
		}},
		Packages: []ci.IntegerPublishedPackage{{Architecture: file.Architecture, Name: file.Name, SHA256: file.SHA256, Artifact: artifact}},
	}
}

func commandDigest(seed string) string {
	return "sha256:" + strings.Repeat(seed, 64)
}
