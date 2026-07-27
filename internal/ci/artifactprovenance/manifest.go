package artifactprovenance

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const manifestVersion = 1

type manifest struct {
	Version       int    `json:"version"`
	Repository    string `json:"repository"`
	RunID         uint64 `json:"run_id"`
	RunAttempt    uint64 `json:"run_attempt"`
	SourceSHA     string `json:"source_sha"`
	PublicationID string `json:"publication_id"`
	ArtifactName  string `json:"artifact_name"`
}

func WriteManifest(path string, identity *Identity) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create provenance manifest %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close provenance manifest %q: %w", path, closeErr)
		}
	}()

	if err := json.NewEncoder(file).Encode(manifestFromIdentity(identity)); err != nil {
		return fmt.Errorf("encode provenance manifest %q: %w", path, err)
	}
	return nil
}

func readManifest(path string) (Identity, error) {
	file, err := os.Open(path)
	if err != nil {
		return Identity{}, fmt.Errorf("open provenance manifest %q: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 64*1024))
	decoder.DisallowUnknownFields()
	var value manifest
	if err := decoder.Decode(&value); err != nil {
		return Identity{}, fmt.Errorf("decode provenance manifest %q: %w", path, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Identity{}, fmt.Errorf("decode provenance manifest %q: %w", path, err)
	}
	if value.Version != manifestVersion {
		return Identity{}, fmt.Errorf("%w: manifest version %d", ErrInvalidProvenance, value.Version)
	}
	input := IdentityInput{
		Repository: value.Repository, RunID: value.RunID, RunAttempt: value.RunAttempt,
		SourceSHA: value.SourceSHA, PublicationID: value.PublicationID, ArtifactName: value.ArtifactName,
	}
	identity, err := ParseIdentity(&input)
	if err != nil {
		return Identity{}, fmt.Errorf("parse provenance manifest identity: %w", err)
	}
	return identity, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("%w: trailing JSON value", ErrInvalidProvenance)
}

func manifestFromIdentity(identity *Identity) manifest {
	return manifest{
		Version: manifestVersion, Repository: identity.repository, RunID: identity.runID,
		RunAttempt: identity.runAttempt, SourceSHA: identity.sourceSHA,
		PublicationID: identity.publicationID, ArtifactName: identity.artifactName,
	}
}
