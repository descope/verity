package publication

import "context"

const SchemaVersion = 1

type (
	SourceSHA     string
	RunID         uint64
	RunAttempt    uint64
	BatchID       string
	Digest        string
	Mode          string
	Event         string
	Result        string
	ComponentKind string
	APKAction     string
	Architecture  string
)

const (
	ModeBootstrap Mode = "bootstrap"
	ModeSnapshot  Mode = "snapshot"
	ModeDelta     Mode = "delta"
	ModeRestore   Mode = "restore"

	EventSchedule         Event = "schedule"
	EventPush             Event = "push"
	EventWorkflowCall     Event = "workflow_call"
	EventWorkflowDispatch Event = "workflow_dispatch"

	ResultSuccess Result = "success"

	ComponentKindAPK     ComponentKind = "apk"
	ComponentKindGeneric ComponentKind = "generic"

	APKUpsert APKAction = "upsert"
	APKRemove APKAction = "remove"

	ArchitectureX8664   Architecture = "x86_64"
	ArchitectureAArch64 Architecture = "aarch64"
)

type Manifest struct {
	SchemaVersion                 int            `json:"schema_version"`
	SourceSHA                     SourceSHA      `json:"source_sha"`
	RunID                         RunID          `json:"run_id"`
	RunAttempt                    RunAttempt     `json:"run_attempt"`
	BatchID                       BatchID        `json:"batch_id"`
	Mode                          Mode           `json:"mode"`
	PreviousManifestDigest        Digest         `json:"previous_manifest_digest,omitempty"`
	Components                    []Component    `json:"components"`
	SignerDigest                  Digest         `json:"signer_digest"`
	SigningKeyEpoch               uint64         `json:"signing_key_epoch,omitempty"`
	ActiveSigningKeyFingerprint   string         `json:"active_signing_key_fingerprint,omitempty"`
	TrustedSigningKeyFingerprints []string       `json:"trusted_signing_key_fingerprints,omitempty"`
	RevokedSigningKeyFingerprints []string       `json:"revoked_signing_key_fingerprints,omitempty"`
	APKOperations                 []APKOperation `json:"apk_operations"`
}

type ProducerIdentity struct {
	SourceSHA  SourceSHA
	RunID      RunID
	RunAttempt RunAttempt
	BatchID    BatchID
}

type Component struct {
	Name           string        `json:"name"`
	Kind           ComponentKind `json:"kind"`
	Architecture   Architecture  `json:"architecture,omitempty"`
	ArtifactName   string        `json:"artifact_name"`
	ArtifactDigest Digest        `json:"artifact_digest"`
	ManifestDigest Digest        `json:"manifest_digest,omitempty"`
	Workflow       string        `json:"workflow"`
	Event          Event         `json:"event"`
	Result         Result        `json:"result"`
}

type APKOperation struct {
	Action         APKAction    `json:"action"`
	Architecture   Architecture `json:"architecture"`
	PackageName    string       `json:"package_name"`
	ArtifactName   string       `json:"artifact_name,omitempty"`
	ArtifactDigest Digest       `json:"artifact_digest,omitempty"`
}

type ValidationOptions struct {
	ExpectedIdentity     ProducerIdentity
	ExpectedMode         Mode
	ExpectedComponents   []Component
	ExpectedSignerDigest Digest
	PublicationSHA       SourceSHA
	PreviousManifest     *Manifest
	AuthorizeBootstrap   bool
	AuthorizeRestore     bool
	RepositoryDir        string
	Runner               Runner
}

type Command struct {
	Name string
	Args []string
}

type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type Runner interface {
	Run(context.Context, Command) (CommandResult, error)
}
