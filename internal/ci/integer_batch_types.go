package ci

import "errors"

const (
	IntegerBatchSchemaVersion    = 1
	IntegerComponentManifestName = "component.json"
)

var (
	ErrIntegerBatchPlan           = errors.New("invalid Integer batch plan")
	ErrIntegerPackageMissing      = errors.New("declared Integer package is missing")
	ErrIntegerPackageDuplicate    = errors.New("duplicate Integer package or component")
	ErrIntegerPackageUndeclared   = errors.New("undeclared Integer package")
	ErrIntegerPackageArchitecture = errors.New("integer package architecture mismatch")
	ErrIntegerIdentityMismatch    = errors.New("integer producer identity mismatch")
	ErrIntegerShardIncomplete     = errors.New("incomplete Integer shard")
	ErrIntegerBatchIncomplete     = errors.New("incomplete Integer batch")
)

type IntegerBatchMode string

const (
	IntegerBatchModeSnapshot IntegerBatchMode = "snapshot"
	IntegerBatchModeDelta    IntegerBatchMode = "delta"
)

type IntegerBatchEvent string

const (
	IntegerBatchEventSchedule         IntegerBatchEvent = "schedule"
	IntegerBatchEventPush             IntegerBatchEvent = "push"
	IntegerBatchEventWorkflowCall     IntegerBatchEvent = "workflow_call"
	IntegerBatchEventWorkflowDispatch IntegerBatchEvent = "workflow_dispatch"
)

type IntegerArchitecture string

const (
	IntegerArchitectureX8664   IntegerArchitecture = "x86_64"
	IntegerArchitectureAArch64 IntegerArchitecture = "aarch64"
)

type IntegerBatchTarget struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Type             string   `json:"type"`
	ArtifactKey      string   `json:"artifact_key"`
	Tags             []string `json:"tags"`
	Registry         string   `json:"registry"`
	Shard            string   `json:"shard"`
	ExpectedPackages []string `json:"expected_packages"`
	PublishPackages  []string `json:"publish_packages"`
}

func (target *IntegerBatchTarget) ID() string {
	return target.Name + ":" + target.Version + "-" + target.Type
}

type IntegerPlannedPackage struct {
	Architecture IntegerArchitecture `json:"architecture"`
	Name         string              `json:"name"`
	Producer     string              `json:"producer"`
}

type IntegerBatchPlan struct {
	SchemaVersion int                     `json:"schema_version"`
	SourceSHA     string                  `json:"source_sha"`
	RunID         uint64                  `json:"run_id"`
	RunAttempt    uint64                  `json:"run_attempt"`
	PublicationID string                  `json:"publication_id"`
	BatchID       string                  `json:"batch_id"`
	Mode          IntegerBatchMode        `json:"mode"`
	Event         IntegerBatchEvent       `json:"event"`
	Targets       []IntegerBatchTarget    `json:"targets"`
	Packages      []IntegerPlannedPackage `json:"packages"`
}

type IntegerPackageFile struct {
	Architecture IntegerArchitecture `json:"architecture"`
	Name         string              `json:"name"`
	SHA256       string              `json:"sha256"`
	Path         string              `json:"path"`
}

type IntegerComponentManifest struct {
	SchemaVersion int                  `json:"schema_version"`
	SourceSHA     string               `json:"source_sha"`
	RunID         uint64               `json:"run_id"`
	RunAttempt    uint64               `json:"run_attempt"`
	PublicationID string               `json:"publication_id"`
	BatchID       string               `json:"batch_id"`
	Mode          IntegerBatchMode     `json:"mode"`
	Event         IntegerBatchEvent    `json:"event"`
	TargetID      string               `json:"target_id"`
	Shard         string               `json:"shard"`
	Packages      []IntegerPackageFile `json:"packages"`
}

type IntegerArtifactRef struct {
	PublicationID string `json:"publication_id"`
	Name          string `json:"name"`
	Digest        string `json:"digest"`
}

type IntegerShardInventory struct {
	SchemaVersion int                  `json:"schema_version"`
	SourceSHA     string               `json:"source_sha"`
	RunID         uint64               `json:"run_id"`
	RunAttempt    uint64               `json:"run_attempt"`
	PublicationID string               `json:"publication_id"`
	BatchID       string               `json:"batch_id"`
	Mode          IntegerBatchMode     `json:"mode"`
	Event         IntegerBatchEvent    `json:"event"`
	Shard         string               `json:"shard"`
	Packages      []IntegerPackageFile `json:"packages"`
}

type IntegerShardManifest struct {
	SchemaVersion int                  `json:"schema_version"`
	SourceSHA     string               `json:"source_sha"`
	RunID         uint64               `json:"run_id"`
	RunAttempt    uint64               `json:"run_attempt"`
	PublicationID string               `json:"publication_id"`
	BatchID       string               `json:"batch_id"`
	Mode          IntegerBatchMode     `json:"mode"`
	Event         IntegerBatchEvent    `json:"event"`
	Shard         string               `json:"shard"`
	Artifact      IntegerArtifactRef   `json:"artifact"`
	Packages      []IntegerPackageFile `json:"packages"`
}

type IntegerPublishedPackage struct {
	Architecture IntegerArchitecture `json:"architecture"`
	Name         string              `json:"name"`
	SHA256       string              `json:"sha256"`
	Artifact     IntegerArtifactRef  `json:"artifact"`
}

type IntegerBatchManifest struct {
	SchemaVersion int                       `json:"schema_version"`
	SourceSHA     string                    `json:"source_sha"`
	RunID         uint64                    `json:"run_id"`
	RunAttempt    uint64                    `json:"run_attempt"`
	PublicationID string                    `json:"publication_id"`
	BatchID       string                    `json:"batch_id"`
	Mode          IntegerBatchMode          `json:"mode"`
	Event         IntegerBatchEvent         `json:"event"`
	Shards        []IntegerShardManifest    `json:"shards"`
	Packages      []IntegerPublishedPackage `json:"packages"`
}

type IntegerProductionOptions struct {
	Event              IntegerBatchEvent
	SourceSHA          string
	RunID              uint64
	RunAttempt         uint64
	PublicationID      string
	BatchID            string
	ChangedFiles       []string
	Only               []string
	PackageTargetsOnly bool
	RepoRoot           string
	BaseLockPath       string
	BaseImagesDir      string
	ConfigPath         string
	ImagesDir          string
	APKIndexURL        string
	CacheDir           string
	GenDir             string
}

type IntegerComponentOptions struct {
	Plan        *IntegerBatchPlan
	TargetID    string
	PackagesDir string
	OutputDir   string
}

type IntegerShardOptions struct {
	Plan          *IntegerBatchPlan
	Shard         string
	ComponentDirs []string
	OutputDir     string
}
