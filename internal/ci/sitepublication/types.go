package sitepublication

import (
	"context"
	"errors"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
)

const (
	SchemaVersion             = 1
	PublicationManifestPath   = ".verity/publication-manifest.json"
	SiteFileManifestPath      = ".verity/site-files.json"
	PagesArtifactName         = "github-pages"
	PublishWorkflow           = ".github/workflows/publish.yaml"
	PublishWorkflowName       = "Publish"
	BuildSiteWorkflow         = ".github/workflows/build-site.yaml"
	BuildSiteWorkflowIdentity = "github.com/verity-org/verity/.github/workflows/build-site.yaml"
)

var (
	ErrInvalidPlan         = errors.New("invalid site publication plan")
	ErrInvalidAssembly     = errors.New("invalid site assembly")
	ErrOverlayConflict     = errors.New("conflicting site overlay mutation")
	ErrUndeclaredMutation  = errors.New("undeclared site mutation")
	ErrInvalidSignerPlan   = errors.New("invalid signer execution plan")
	ErrSignerExecution     = errors.New("signer execution failed")
	ErrArtifactTampered    = errors.New("site artifact digest mismatch")
	ErrInvalidArchive      = errors.New("invalid deterministic site archive")
	ErrInvalidFinalPlan    = errors.New("invalid final publication plan")
	ErrUnsupportedSignMode = errors.New("publication mode cannot be signed")
)

type PlanRequest struct {
	Manifest                publication.Manifest
	ExpectedIdentity        publication.ProducerIdentity
	ExpectedMode            publication.Mode
	ExpectedComponents      []publication.Component
	PublicationSHA          publication.SourceSHA
	PreviousManifest        *publication.Manifest
	SignerLock              signerlock.Lock
	ExpectedSignerSourceSHA string
	AuthorizeBootstrap      bool
	AuthorizeRestore        bool
	RepositoryDir           string
	Runner                  publication.Runner
}

type PublicationPlan struct {
	SchemaVersion          int                    `json:"schema_version"`
	ManifestDigest         publication.Digest     `json:"manifest_digest"`
	PreviousManifestDigest publication.Digest     `json:"previous_manifest_digest,omitempty"`
	Mode                   publication.Mode       `json:"mode"`
	SourceSHA              publication.SourceSHA  `json:"source_sha"`
	RunID                  publication.RunID      `json:"run_id"`
	RunAttempt             publication.RunAttempt `json:"run_attempt"`
	BatchID                publication.BatchID    `json:"batch_id"`
	SignerDigest           publication.Digest     `json:"signer_digest"`
	SignerSourceSHA        publication.SourceSHA  `json:"signer_source_sha"`
	SignerReference        string                 `json:"signer_reference"`
	PlanDigest             publication.Digest     `json:"plan_digest"`
}

type Overlay struct {
	Name        string
	SourceDir   string
	Destination string
}

type AssembleRequest struct {
	Plan         PublicationPlan
	Manifest     publication.Manifest
	BaseDir      string
	SignedAPKDir string
	OutputDir    string
	Overlays     []Overlay
}

type AssemblyResult struct {
	ManifestDigest publication.Digest `json:"manifest_digest"`
	SiteDigest     publication.Digest `json:"site_digest"`
	FileCount      int                `json:"file_count"`
}

type SiteFile struct {
	Path   string             `json:"path"`
	SHA256 publication.Digest `json:"sha256"`
	Mode   uint32             `json:"mode"`
}

type SiteFileManifest struct {
	SchemaVersion int        `json:"schema_version"`
	Files         []SiteFile `json:"files"`
}

type VerifiedSite struct {
	Manifest       publication.Manifest
	ManifestDigest publication.Digest
	SiteDigest     publication.Digest
	FileCount      int
}

type AttestationPlan struct {
	SubjectPath   string                `json:"subject_path"`
	SubjectDigest publication.Digest    `json:"subject_digest"`
	Workflow      string                `json:"workflow"`
	SourceSHA     publication.SourceSHA `json:"source_sha"`
}

type FinalPlan struct {
	SchemaVersion  int                    `json:"schema_version"`
	ArtifactName   string                 `json:"artifact_name"`
	ArtifactPath   string                 `json:"artifact_path"`
	ArtifactDigest publication.Digest     `json:"artifact_digest"`
	ManifestDigest publication.Digest     `json:"manifest_digest"`
	SiteDigest     publication.Digest     `json:"site_digest"`
	RunID          publication.RunID      `json:"run_id"`
	RunAttempt     publication.RunAttempt `json:"run_attempt"`
	Attestation    AttestationPlan        `json:"attestation"`
	DeployEligible bool                   `json:"deploy_eligible"`
}

type FinalizeRequest struct {
	Plan               PublicationPlan
	ExpectedPlanDigest publication.Digest
	SiteDir            string
	ArchivePath        string
	CurrentManifest    *publication.Manifest
}

type SignerRequest struct {
	Plan              PublicationPlan
	Runtime           string
	Repository        string
	WorkspaceDir      string
	KeyDirectory      string
	ManifestPath      string
	PackagesPath      string
	BaseAPKPath       string
	DeltaManifestPath string
	OutputAPKPath     string
	PublicKeyPath     string
}

type ContainerRuntime string

const (
	ContainerRuntimeDocker ContainerRuntime = "docker"
	ContainerRuntimePodman ContainerRuntime = "podman"
)

type SignerExecutionSpec struct {
	Runtime           ContainerRuntime   `json:"runtime"`
	Mode              publication.Mode   `json:"mode"`
	Repository        string             `json:"repository"`
	WorkspaceDir      string             `json:"workspace_dir"`
	PathSnapshot      publication.Digest `json:"path_snapshot"`
	ManifestPath      string             `json:"manifest_path"`
	PackagesPath      string             `json:"packages_path"`
	BaseAPKPath       string             `json:"base_apk_path,omitempty"`
	DeltaManifestPath string             `json:"delta_manifest_path,omitempty"`
	OutputAPKPath     string             `json:"output_apk_path"`
	PublicKeyPath     string             `json:"public_key_path"`
}

type ExecutionCommand struct {
	Name  string   `json:"name"`
	Args  []string `json:"args"`
	Stdin []byte   `json:"-"`
}

type ExecutionResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type ExecutionRunner interface {
	Run(context.Context, ExecutionCommand) (ExecutionResult, error)
}

type SignerStep struct {
	Name      string           `json:"name"`
	KeyAccess bool             `json:"key_access"`
	Command   ExecutionCommand `json:"command"`
}

type KeyCleanup struct {
	KeyDirectory string `json:"key_directory"`
	KeyPath      string `json:"key_path"`
}

type SignerPlan struct {
	SchemaVersion         int                      `json:"schema_version"`
	PublicationPlanDigest publication.Digest       `json:"publication_plan_digest"`
	ManifestDigest        publication.Digest       `json:"manifest_digest"`
	SignerDigest          publication.Digest       `json:"signer_digest"`
	SignerSourceSHA       publication.SourceSHA    `json:"signer_source_sha"`
	ImageReference        string                   `json:"image_reference"`
	InputDigest           publication.Digest       `json:"input_digest"`
	Authorization         SignerInputAuthorization `json:"authorization"`
	Execution             SignerExecutionSpec      `json:"execution"`
	Steps                 []SignerStep             `json:"steps"`
	Cleanup               KeyCleanup               `json:"cleanup"`
}

type SignerAuthorizedInput struct {
	Path           string             `json:"path"`
	ContentDigest  publication.Digest `json:"content_digest"`
	SemanticDigest publication.Digest `json:"semantic_digest,omitempty"`
}

type SignerInputAuthorization struct {
	SchemaVersion         int                        `json:"schema_version"`
	PublicationPlanDigest publication.Digest         `json:"publication_plan_digest"`
	ManifestDigest        publication.Digest         `json:"manifest_digest"`
	Mode                  publication.Mode           `json:"mode"`
	ManifestPath          string                     `json:"manifest_path"`
	PackagesPath          string                     `json:"packages_path"`
	BaseAPKPath           string                     `json:"base_apk_path,omitempty"`
	DeltaManifestPath     string                     `json:"delta_manifest_path,omitempty"`
	PublicKeyPath         string                     `json:"public_key_path"`
	APKOperations         []publication.APKOperation `json:"apk_operations"`
	Inputs                []SignerAuthorizedInput    `json:"inputs"`
	Packages              []SignerAuthorizedInput    `json:"packages"`
}

type SignerResult struct {
	SignerDigest publication.Digest `json:"signer_digest"`
	OutputPath   string             `json:"output_path"`
	OutputDigest publication.Digest `json:"output_digest"`
	Signed       bool               `json:"signed"`
	KeyCleaned   bool               `json:"key_cleaned"`
}

type SignerOperationResult struct {
	OutputPath   string             `json:"output_path"`
	OutputDigest publication.Digest `json:"output_digest"`
}
