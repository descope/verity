package publication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	ci "github.com/verity-org/verity/internal/ci"
)

const producerManifestVersion = 1

type producerSet struct {
	integer    ci.IntegerBatchManifest
	components []Component
}

type artifactProducerManifest struct {
	Version       int    `json:"version"`
	Repository    string `json:"repository"`
	RunID         uint64 `json:"run_id"`
	RunAttempt    uint64 `json:"run_attempt"`
	SourceSHA     string `json:"source_sha"`
	PublicationID string `json:"publication_id"`
	ArtifactName  string `json:"artifact_name"`
}

func parseProducerSet(request *ComposeRequest) (producerSet, error) {
	inputs := make(map[string]ProducerManifestInput, len(request.Producers))
	for _, input := range request.Producers {
		switch input.Name {
		case "integer", "charts", "site":
		default:
			return producerSet{}, fmt.Errorf("%w: %q", ErrProducerUndeclared, input.Name)
		}
		if _, exists := inputs[input.Name]; exists {
			return producerSet{}, fmt.Errorf("%w: %q", ErrProducerDuplicate, input.Name)
		}
		inputs[input.Name] = input
	}
	for _, required := range []string{"integer", "charts"} {
		if _, exists := inputs[required]; !exists {
			return producerSet{}, fmt.Errorf("%w: %q", ErrProducerMissing, required)
		}
	}
	integer, components, publicationID, err := parseIntegerProducer(request, inputs["integer"])
	if err != nil {
		return producerSet{}, err
	}
	for _, name := range []string{"charts", "site"} {
		input, exists := inputs[name]
		if !exists {
			continue
		}
		component, err := parseArtifactProducer(request, publicationID, input)
		if err != nil {
			return producerSet{}, err
		}
		components = append(components, component)
	}
	return producerSet{integer: integer, components: components}, nil
}

func parseIntegerProducer(request *ComposeRequest, input ProducerManifestInput) (ci.IntegerBatchManifest, []Component, string, error) {
	manifest, err := ci.ParseIntegerBatchManifest(input.Data)
	if err != nil {
		return ci.IntegerBatchManifest{}, nil, "", fmt.Errorf("%w: integer: %w", ErrProducerConflict, err)
	}
	canonical, err := ci.MarshalIntegerBatchManifest(&manifest)
	if err != nil || !bytes.Equal(canonical, input.Data) {
		return ci.IntegerBatchManifest{}, nil, "", fmt.Errorf("%w: integer manifest is not canonical", ErrProducerConflict)
	}
	if err := correlateIntegerIdentity(request, &manifest, input); err != nil {
		return ci.IntegerBatchManifest{}, nil, "", err
	}
	components := []Component{{
		Name: "integer", Kind: ComponentKindGeneric, ArtifactName: input.ArtifactName,
		ArtifactDigest: input.ArtifactDigest, ManifestDigest: digestData(input.Data),
		Workflow: ".github/workflows/integer-orchestrator-reusable.yaml",
		Event:    EventWorkflowCall, Result: ResultSuccess,
	}}
	for shardIndex := range manifest.Shards {
		shard := &manifest.Shards[shardIndex]
		shardData, marshalErr := ci.MarshalIntegerShardManifest(shard)
		if marshalErr != nil {
			return ci.IntegerBatchManifest{}, nil, "", fmt.Errorf("%w: integer shard %q: %w", ErrProducerConflict, shard.Shard, marshalErr)
		}
		architectures := make([]Architecture, 0, 2)
		for _, pkg := range shard.Packages {
			architecture := Architecture(pkg.Architecture)
			if !slices.Contains(architectures, architecture) {
				architectures = append(architectures, architecture)
			}
		}
		slices.Sort(architectures)
		for _, architecture := range architectures {
			components = append(components, Component{
				Name: "integer-" + shard.Shard + "-" + string(architecture), Kind: ComponentKindAPK,
				Architecture: architecture, ArtifactName: shard.Artifact.Name,
				ArtifactDigest: Digest(shard.Artifact.Digest), ManifestDigest: digestData(shardData),
				Workflow: ".github/workflows/integer-build-shard.yaml",
				Event:    mapIntegerEvent(manifest.Event), Result: ResultSuccess,
			})
		}
	}
	return manifest, components, manifest.PublicationID, nil
}

func correlateIntegerIdentity(request *ComposeRequest, manifest *ci.IntegerBatchManifest, input ProducerManifestInput) error {
	expectedPublicationID := fmt.Sprintf("build-site-%d-%d", request.Identity.RunID, request.Identity.RunAttempt)
	expectedArtifact := "integer-manifest-" + manifest.PublicationID
	if manifest.SourceSHA != string(request.Identity.SourceSHA) || manifest.RunID != uint64(request.Identity.RunID) ||
		manifest.RunAttempt != uint64(request.Identity.RunAttempt) || manifest.BatchID != string(request.Identity.BatchID) ||
		manifest.PublicationID != expectedPublicationID || manifest.Event != ci.IntegerBatchEventWorkflowCall {
		return fmt.Errorf("%w: integer", ErrProducerIdentity)
	}
	if input.ArtifactName != expectedArtifact || !digestPattern.MatchString(string(input.ArtifactDigest)) {
		return fmt.Errorf("%w: integer artifact", ErrProducerConflict)
	}
	switch request.Mode {
	case ModeBootstrap, ModeSnapshot:
		if manifest.Mode != ci.IntegerBatchModeSnapshot {
			return fmt.Errorf("%w: integer mode", ErrProducerConflict)
		}
	case ModeDelta:
		if manifest.Mode != ci.IntegerBatchModeDelta {
			return fmt.Errorf("%w: integer mode", ErrProducerConflict)
		}
	default:
		return fmt.Errorf("%w: unsupported compose mode %q", ErrComposeInvalid, request.Mode)
	}
	return nil
}

func parseArtifactProducer(request *ComposeRequest, publicationID string, input ProducerManifestInput) (Component, error) {
	var manifest artifactProducerManifest
	if err := decodeStrictProducerJSON(input.Data, &manifest); err != nil {
		return Component{}, fmt.Errorf("%w: %s: %w", ErrProducerConflict, input.Name, err)
	}
	canonical, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(append(canonical, '\n'), input.Data) {
		return Component{}, fmt.Errorf("%w: %s manifest is not canonical", ErrProducerConflict, input.Name)
	}
	if manifest.Version != producerManifestVersion || manifest.Repository != "verity-org/verity" ||
		manifest.SourceSHA != string(request.Identity.SourceSHA) || manifest.RunID != uint64(request.Identity.RunID) ||
		manifest.RunAttempt != uint64(request.Identity.RunAttempt) || manifest.PublicationID != publicationID {
		return Component{}, fmt.Errorf("%w: %s", ErrProducerIdentity, input.Name)
	}
	if manifest.ArtifactName != input.ArtifactName || !digestPattern.MatchString(string(input.ArtifactDigest)) {
		return Component{}, fmt.Errorf("%w: %s artifact", ErrProducerConflict, input.Name)
	}
	workflow := ".github/workflows/chart-gen.yaml"
	if input.Name == "site" {
		workflow = ".github/workflows/build-site-catalog.yaml"
	}
	return Component{
		Name: input.Name, Kind: ComponentKindGeneric, ArtifactName: input.ArtifactName,
		ArtifactDigest: input.ArtifactDigest, ManifestDigest: digestData(input.Data),
		Workflow: workflow, Event: EventWorkflowCall, Result: ResultSuccess,
	}, nil
}

func decodeStrictProducerJSON(data []byte, destination any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errTrailingJSONValue
	}
	return nil
}

func mapIntegerEvent(event ci.IntegerBatchEvent) Event {
	return Event(event)
}

func digestData(data []byte) Digest {
	sum := sha256.Sum256(data)
	return Digest("sha256:" + hex.EncodeToString(sum[:]))
}
