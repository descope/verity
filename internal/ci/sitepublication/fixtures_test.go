package sitepublication

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
)

const (
	testSourceSHA       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testPreviousSHA     = "9999999999999999999999999999999999999999"
	testSignerSourceSHA = "cccccccccccccccccccccccccccccccccccccccc"
	testSignerDigest    = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

type acceptingPublicationRunner struct{}

func (acceptingPublicationRunner) Run(context.Context, publication.Command) (publication.CommandResult, error) {
	return publication.CommandResult{}, nil
}

func testComponents() []publication.Component {
	return []publication.Component{
		{
			Name: "catalog", Kind: publication.ComponentKindGeneric,
			ArtifactName: "catalog-42-3", ArtifactDigest: digestOf("1"),
			Workflow: ".github/workflows/build-site-catalog.yaml",
			Event:    publication.EventWorkflowCall, Result: publication.ResultSuccess,
		},
		{
			Name: "charts", Kind: publication.ComponentKindGeneric,
			ArtifactName: "charts-42-3", ArtifactDigest: digestOf("2"),
			Workflow: ".github/workflows/chart-gen.yaml",
			Event:    publication.EventWorkflowCall, Result: publication.ResultSuccess,
		},
		{
			Name: "integer-x86", Kind: publication.ComponentKindAPK,
			Architecture: publication.ArchitectureX8664,
			ArtifactName: "integer-42-3-x86", ArtifactDigest: digestOf("3"),
			Workflow: ".github/workflows/integer-build-image.yaml",
			Event:    publication.EventWorkflowCall, Result: publication.ResultSuccess,
		},
	}
}

func testManifest(mode publication.Mode, source string, runID, attempt uint64) publication.Manifest {
	return publication.Manifest{
		SchemaVersion: publication.SchemaVersion,
		SourceSHA:     publication.SourceSHA(source),
		RunID:         publication.RunID(runID),
		RunAttempt:    publication.RunAttempt(attempt),
		BatchID:       publication.BatchID(batchID(runID, attempt)),
		Mode:          mode,
		Components:    testComponents(),
		SignerDigest:  testSignerDigest,
		APKOperations: []publication.APKOperation{{
			Action: publication.APKUpsert, Architecture: publication.ArchitectureX8664,
			PackageName: "demo", ArtifactName: "integer-42-3-x86", ArtifactDigest: digestOf("3"),
		}},
	}
}

func validPlanRequest(t *testing.T) *PlanRequest {
	t.Helper()
	previous := testManifest(publication.ModeBootstrap, testPreviousSHA, 41, 1)
	previousDigest, err := publication.DigestManifest(&previous)
	require.NoError(t, err)
	candidate := testManifest(publication.ModeDelta, testSourceSHA, 42, 3)
	candidate.PreviousManifestDigest = previousDigest
	return &PlanRequest{
		Manifest:                candidate,
		ExpectedIdentity:        publication.ProducerIdentity{SourceSHA: candidate.SourceSHA, RunID: candidate.RunID, RunAttempt: candidate.RunAttempt, BatchID: candidate.BatchID},
		ExpectedMode:            candidate.Mode,
		ExpectedComponents:      testComponents(),
		PublicationSHA:          candidate.SourceSHA,
		PreviousManifest:        &previous,
		SignerLock:              validSignerLock(),
		ExpectedSignerSourceSHA: testSignerSourceSHA,
		Runner:                  acceptingPublicationRunner{},
	}
}

func validSignerLock() signerlock.Lock {
	return signerlock.Lock{
		Image: signerlock.SignerImageRepository, Digest: testSignerDigest,
		Workflow: signerlock.TrustedWorkflowIdentity, SourceSHA: testSignerSourceSHA,
		Runnable: true,
	}
}

func digestOf(character string) publication.Digest {
	return publication.Digest("sha256:" + strings.Repeat(character, 64))
}

func batchID(runID, attempt uint64) string {
	return fmtUint(runID) + "-" + fmtUint(attempt)
}

func fmtUint(value uint64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = digits[value%10]
		value /= 10
	}
	return string(buffer[position:])
}
