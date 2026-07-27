package apkrepository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/sitepublication"
)

const (
	trustedPreviousRepository = "verity-org/verity"
	buildSiteBranch           = "main"
)

var (
	errPreviousRunNotFound       = errors.New("previous successful Build Site run not found")
	errAmbiguousPreviousRun      = errors.New("ambiguous previous Build Site run")
	errExpiredPagesArtifact      = errors.New("previous Build Site Pages artifact is expired")
	errUntrustedPreviousResolver = errors.New("untrusted previous-publication resolver input")
)

type ResolvePreviousOptions struct {
	Repository   string
	Workflow     string
	Branch       string
	ArtifactName string
	BeforeRunID  uint64
	runner       commandRunner
}

type PreviousPublication struct {
	RunID          uint64
	RunAttempt     uint64
	SourceSHA      string
	ArtifactDigest string
	ManifestDigest string
}

type priorWorkflowRunList struct {
	WorkflowRuns []priorWorkflowRun `json:"workflow_runs"`
}

type previousPublicationResolver struct {
	ctx     context.Context
	runner  commandRunner
	options *ResolvePreviousOptions
}

func ResolvePrevious(ctx context.Context, options *ResolvePreviousOptions) (PreviousPublication, error) {
	if err := validateResolvePreviousOptions(options); err != nil {
		return PreviousPublication{}, err
	}
	runner := options.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	resolver := previousPublicationResolver{ctx: ctx, runner: runner, options: options}
	run, err := resolver.latestEarlierRun()
	if err != nil {
		return PreviousPublication{}, err
	}
	artifact, err := resolver.exactArtifact(&run)
	if err != nil {
		return PreviousPublication{}, err
	}
	return resolver.verifyArtifact(&run, &artifact)
}

func validateResolvePreviousOptions(options *ResolvePreviousOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	if options.Repository != trustedPreviousRepository || options.Workflow != sitepublication.PublishWorkflow ||
		options.Branch != buildSiteBranch || options.ArtifactName != sitepublication.PagesArtifactName || options.BeforeRunID == 0 {
		return errUntrustedPreviousResolver
	}
	return nil
}

func (resolver *previousPublicationResolver) latestEarlierRun() (priorWorkflowRun, error) {
	endpoint := "repos/" + resolver.options.Repository + "/actions/workflows/" + url.PathEscape(resolver.options.Workflow) + "/runs"
	result, err := runRequired(resolver.ctx, resolver.runner, &command{
		name: "gh",
		args: []string{
			"api", endpoint, "--method", "GET",
			"-f", "branch=" + resolver.options.Branch,
			"-f", "status=success", "-f", "per_page=100",
			"--paginate", "--slurp",
		},
	})
	if err != nil {
		return priorWorkflowRun{}, fmt.Errorf("list prior Build Site runs: %w", err)
	}
	var pages []priorWorkflowRunList
	if err := json.Unmarshal(result.stdout, &pages); err != nil {
		return priorWorkflowRun{}, fmt.Errorf("decode prior Build Site runs: %w", err)
	}
	return selectLatestEarlierRun(pages, resolver.options)
}

func selectLatestEarlierRun(pages []priorWorkflowRunList, options *ResolvePreviousOptions) (priorWorkflowRun, error) {
	selected, matches := latestEarlierRunCandidate(pages, options.BeforeRunID)
	if matches == 0 {
		return priorWorkflowRun{}, errPreviousRunNotFound
	}
	if matches != 1 {
		return priorWorkflowRun{}, errAmbiguousPreviousRun
	}
	if err := validateResolvedRun(&selected, options); err != nil {
		return priorWorkflowRun{}, err
	}
	return selected, nil
}

func latestEarlierRunCandidate(pages []priorWorkflowRunList, beforeRunID uint64) (selected priorWorkflowRun, matches int) {
	for _, page := range pages {
		for _, run := range page.WorkflowRuns {
			if run.ID >= beforeRunID {
				continue
			}
			if matches == 0 || run.ID > selected.ID {
				selected = run
				matches = 1
				continue
			}
			if run.ID == selected.ID {
				matches++
			}
		}
	}
	return selected, matches
}

func validateResolvedRun(run *priorWorkflowRun, options *ResolvePreviousOptions) error {
	if run.Path != options.Workflow || run.Name != sitepublication.PublishWorkflowName {
		return errWrongPagesWorkflow
	}
	if run.ID == 0 || run.RunAttempt == 0 || run.HeadBranch != options.Branch ||
		!restoreSHA.MatchString(run.HeadSHA) || run.Conclusion != "success" {
		return errWrongPagesRun
	}
	return nil
}

func (resolver *previousPublicationResolver) exactArtifact(run *priorWorkflowRun) (priorArtifact, error) {
	endpoint := "repos/" + resolver.options.Repository + "/actions/runs/" + strconv.FormatUint(run.ID, 10) + "/artifacts"
	result, err := runRequired(resolver.ctx, resolver.runner, &command{
		name: "gh",
		args: []string{
			"api", endpoint, "--method", "GET", "-f", "per_page=100", "--paginate", "--slurp",
		},
	})
	if err != nil {
		return priorArtifact{}, fmt.Errorf("list prior Build Site artifacts: %w", err)
	}
	var pages []priorArtifactList
	if err := json.Unmarshal(result.stdout, &pages); err != nil {
		return priorArtifact{}, fmt.Errorf("decode prior Build Site artifacts: %w", err)
	}
	var matches []priorArtifact
	for _, page := range pages {
		for _, artifact := range page.Artifacts {
			if artifact.Name == resolver.options.ArtifactName {
				matches = append(matches, artifact)
			}
		}
	}
	if len(matches) == 0 {
		return priorArtifact{}, errPagesArtifactNotFound
	}
	if len(matches) != 1 {
		return priorArtifact{}, errAmbiguousPagesArtifact
	}
	artifact := matches[0]
	if artifact.Expired {
		return priorArtifact{}, errExpiredPagesArtifact
	}
	if artifact.ID <= 0 || !sha256Digest.MatchString(artifact.Digest) {
		return priorArtifact{}, errLegacyPagesArtifact
	}
	if artifact.Workflow.ID != run.ID || artifact.Workflow.HeadBranch != run.HeadBranch || artifact.Workflow.HeadSHA != run.HeadSHA {
		return priorArtifact{}, errWrongPagesRun
	}
	return artifact, nil
}

func (resolver *previousPublicationResolver) verifyArtifact(run *priorWorkflowRun, artifact *priorArtifact) (PreviousPublication, error) {
	temporaryDir, err := os.MkdirTemp("", "verity-previous-publication-")
	if err != nil {
		return PreviousPublication{}, fmt.Errorf("create previous-publication directory: %w", err)
	}
	defer os.RemoveAll(temporaryDir)
	zipPath := filepath.Join(temporaryDir, "artifact.zip")
	if err := downloadPriorArtifact(resolver.ctx, resolver.runner, resolver.options.Repository, artifact, zipPath); err != nil {
		return PreviousPublication{}, err
	}
	archivePath := filepath.Join(temporaryDir, "artifact.tar")
	if err := extractArtifactTar(zipPath, archivePath); err != nil {
		return PreviousPublication{}, err
	}
	archiveDigest, err := fileSHA256(archivePath)
	if err != nil {
		return PreviousPublication{}, err
	}
	if err := verifyBuildSiteAttestation(resolver.ctx, resolver.runner, &buildSiteAttestationRequest{
		repository: resolver.options.Repository, signerWorkflow: sitepublication.BuildSiteWorkflowIdentity,
		sourceRef: "refs/heads/" + resolver.options.Branch, sourceDigest: run.HeadSHA, archivePath: archivePath,
	}); err != nil {
		return PreviousPublication{}, err
	}
	verified, err := validateCanonicalPreviousArchive(archivePath, publication.Digest(archiveDigest))
	if err != nil {
		return PreviousPublication{}, err
	}
	if verified.Manifest.SourceSHA != publication.SourceSHA(run.HeadSHA) || uint64(verified.Manifest.RunID) != run.ID ||
		uint64(verified.Manifest.RunAttempt) != run.RunAttempt {
		return PreviousPublication{}, errWrongPagesRun
	}
	return PreviousPublication{
		RunID: run.ID, RunAttempt: run.RunAttempt, SourceSHA: run.HeadSHA,
		ArtifactDigest: archiveDigest, ManifestDigest: string(verified.ManifestDigest),
	}, nil
}
