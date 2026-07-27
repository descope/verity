package publication

import "errors"

var (
	ErrComposeInvalid     = errors.New("invalid publication composition")
	ErrProducerMissing    = errors.New("required producer manifest is missing")
	ErrProducerDuplicate  = errors.New("duplicate producer manifest")
	ErrProducerConflict   = errors.New("conflicting producer manifest")
	ErrProducerUndeclared = errors.New("undeclared producer manifest")
	ErrProducerIdentity   = errors.New("producer manifest identity mismatch")
)

type ProducerManifestInput struct {
	Name           string
	ArtifactName   string
	ArtifactDigest Digest
	Data           []byte
}

type ComposeRequest struct {
	Identity                      ProducerIdentity
	Mode                          Mode
	PreviousManifest              *Manifest
	PreviousManifestDigest        Digest
	SignerDigest                  Digest
	PublicationSHA                SourceSHA
	APKOperations                 []APKOperation
	APKDelta                      []byte
	SigningKeyEpoch               uint64
	ActiveSigningKeyFingerprint   string
	TrustedSigningKeyFingerprints []string
	RevokedSigningKeyFingerprints []string
	AuthorizeBootstrap            bool
	AuthorizeRestore              bool
	RepositoryDir                 string
	Runner                        Runner
	Producers                     []ProducerManifestInput
}

type ComposeResult struct {
	Manifest        Manifest
	PublicationJSON []byte
	ComponentsJSON  []byte
}
