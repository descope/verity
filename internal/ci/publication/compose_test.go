package publication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	ci "github.com/verity-org/verity/internal/ci"
)

func TestCompose_emits_deterministic_canonical_publication_from_exact_producers(t *testing.T) {
	// Given exact current Integer aggregate and chart provenance artifacts.
	integerData := composeIntegerManifest(t)
	chartData := composeChartManifest("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	request := composeRequest(integerData, chartData)

	// When producer order changes across otherwise identical compositions.
	first, err := Compose(context.Background(), &request)
	require.NoError(t, err)
	request.Producers[0], request.Producers[1] = request.Producers[1], request.Producers[0]
	second, err := Compose(context.Background(), &request)
	require.NoError(t, err)

	// Then canonical publication and component bytes are stable and preserve identities.
	require.Equal(t, first.PublicationJSON, second.PublicationJSON)
	require.Equal(t, first.ComponentsJSON, second.ComponentsJSON)
	require.Len(t, first.Manifest.Components, 4)
	require.Contains(t, string(first.PublicationJSON), `"artifact_name":"chart-publication-build-site-42-3"`)
	require.Contains(t, string(first.PublicationJSON), `"manifest_digest":"`+string(digestBytes(chartData))+`"`)
	require.NotContains(t, string(first.PublicationJSON), "\n")
}

func composeRequest(integerData, chartData []byte) ComposeRequest {
	return ComposeRequest{
		Identity: ProducerIdentity{
			SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RunID:     42, RunAttempt: 3, BatchID: "42-3",
		},
		Mode: ModeBootstrap, SignerDigest: digestSeed("9"),
		PublicationSHA:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AuthorizeBootstrap: true,
		Producers: []ProducerManifestInput{
			{Name: "integer", ArtifactName: "integer-manifest-build-site-42-3", ArtifactDigest: digestSeed("1"), Data: integerData},
			{Name: "charts", ArtifactName: "chart-publication-build-site-42-3", ArtifactDigest: digestSeed("2"), Data: chartData},
		},
		Runner: &fakeRunner{result: CommandResult{ExitCode: 0}},
	}
}

func composeIntegerManifest(t *testing.T) []byte {
	t.Helper()
	manifest := ci.IntegerBatchManifest{
		SchemaVersion: ci.IntegerBatchSchemaVersion,
		SourceSHA:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RunID:         42, RunAttempt: 3, PublicationID: "build-site-42-3", BatchID: "42-3",
		Mode: ci.IntegerBatchModeSnapshot, Event: ci.IntegerBatchEventWorkflowCall,
		Shards: []ci.IntegerShardManifest{{
			SchemaVersion: ci.IntegerBatchSchemaVersion,
			SourceSHA:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RunID:         42, RunAttempt: 3, PublicationID: "build-site-42-3", BatchID: "42-3",
			Mode: ci.IntegerBatchModeSnapshot, Event: ci.IntegerBatchEventWorkflowCall, Shard: "1",
			Artifact: ci.IntegerArtifactRef{PublicationID: "build-site-42-3", Name: "apk-repository-build-site-42-3-1", Digest: string(digestSeed("3"))},
			Packages: []ci.IntegerPackageFile{
				{Architecture: ci.IntegerArchitectureX8664, Name: "demo", SHA256: string(digestSeed("4")), Path: "x86_64/demo.apk"},
				{Architecture: ci.IntegerArchitectureAArch64, Name: "demo", SHA256: string(digestSeed("5")), Path: "aarch64/demo.apk"},
			},
		}},
		Packages: []ci.IntegerPublishedPackage{
			{Architecture: ci.IntegerArchitectureX8664, Name: "demo", SHA256: string(digestSeed("4")), Artifact: ci.IntegerArtifactRef{PublicationID: "build-site-42-3", Name: "apk-repository-build-site-42-3-1", Digest: string(digestSeed("3"))}},
			{Architecture: ci.IntegerArchitectureAArch64, Name: "demo", SHA256: string(digestSeed("5")), Artifact: ci.IntegerArtifactRef{PublicationID: "build-site-42-3", Name: "apk-repository-build-site-42-3-1", Digest: string(digestSeed("3"))}},
		},
	}
	data, err := ci.MarshalIntegerBatchManifest(&manifest)
	require.NoError(t, err)
	return data
}

func composeChartManifest(sourceSHA string) []byte {
	return []byte(`{"version":1,"repository":"verity-org/verity","run_id":42,"run_attempt":3,"source_sha":"` + sourceSHA + `","publication_id":"build-site-42-3","artifact_name":"chart-publication-build-site-42-3"}` + "\n")
}

func digestSeed(seed string) Digest {
	return Digest("sha256:" + strings.Repeat(seed, 64))
}

func digestBytes(data []byte) Digest {
	sum := sha256.Sum256(data)
	return Digest("sha256:" + hex.EncodeToString(sum[:]))
}
