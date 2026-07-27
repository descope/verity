package sitepublication

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinalizePublication_rejects_archive_symlink_into_site(t *testing.T) {
	// Given a valid site and an archive parent symlink resolving into that site.
	plan, _, site := assembledFixture(t)
	alias := filepath.Join(filepath.Dir(site), "archive-alias")
	require.NoError(t, os.Symlink(site, alias))
	archive := filepath.Join(alias, "artifact.tar")

	// When finalization reaches the deploy gate.
	finalPlan, err := FinalizePublication(context.Background(), &FinalizeRequest{
		Plan: plan, ExpectedPlanDigest: plan.PlanDigest, SiteDir: site,
		ArchivePath: archive, CurrentManifest: validPlanRequest(t).PreviousManifest,
	})

	// Then no eligible plan or undeclared in-site archive is produced.
	require.ErrorIs(t, err, ErrInvalidArchive)
	assert.False(t, finalPlan.DeployEligible)
	assert.NoFileExists(t, filepath.Join(site, "artifact.tar"))
}

func TestValidateArchive_rejects_symlinked_artifact_path(t *testing.T) {
	// Given a valid artifact referenced through a final-component symlink.
	plan, _, site := assembledFixture(t)
	archive := filepath.Join(filepath.Dir(site), "artifact.tar")
	digest, err := PackSite(site, archive)
	require.NoError(t, err)
	alias := filepath.Join(filepath.Dir(site), "artifact-alias.tar")
	require.NoError(t, os.Symlink(archive, alias))

	// When the alias is offered as the attested artifact.
	_, err = ValidateArchive(alias, digest, plan.ManifestDigest)

	// Then validation never follows the link, even to an in-scope artifact.
	require.ErrorIs(t, err, ErrInvalidArchive)
}

func TestPackSite_rejects_archive_path_traversal_syntax(t *testing.T) {
	// Given a lexical traversal that resolves to an external archive directory.
	_, _, site := assembledFixture(t)
	archive := filepath.Dir(site) + string(filepath.Separator) + "unused/../artifact.tar"

	// When packing receives the non-canonical target.
	_, err := PackSite(site, archive)

	// Then traversal syntax is rejected before any write.
	require.ErrorIs(t, err, ErrInvalidArchive)
	assert.NoFileExists(t, filepath.Join(filepath.Dir(site), "artifact.tar"))
}
