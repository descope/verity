package chartresult

import "errors"

var (
	ErrInvalidResults  = errors.New("invalid chart integration results")
	ErrInvalidIdentity = errors.New("invalid chart integration identity")
)

type IdentityInput struct {
	SourceSHA      string
	RunID          string
	RunAttempt     string
	PublicationID  string
	BatchID        string
	ArtifactName   string
	ArtifactDigest string
}

type Input struct {
	Profile  string
	Results  []string
	Identity IdentityInput
}

type Result struct {
	SourceSHA      string
	RunID          string
	RunAttempt     string
	PublicationID  string
	BatchID        string
	ArtifactName   string
	ArtifactDigest string
}

type Output struct {
	Name  string
	Value string
}

func (result *Result) Outputs() []Output {
	return []Output{
		{Name: "source_sha", Value: result.SourceSHA},
		{Name: "run_id", Value: result.RunID},
		{Name: "run_attempt", Value: result.RunAttempt},
		{Name: "publication_id", Value: result.PublicationID},
		{Name: "batch_id", Value: result.BatchID},
		{Name: "artifact_name", Value: result.ArtifactName},
		{Name: "artifact_digest", Value: result.ArtifactDigest},
	}
}
