package publication

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	ci "github.com/verity-org/verity/internal/ci"
)

type apkDeltaManifest struct {
	FormatVersion    int                 `json:"format_version"`
	BaseSHA256       Digest              `json:"base_sha256"`
	RepositoryFormat string              `json:"repository_format"`
	KeySHA256        Digest              `json:"key_sha256"`
	Operations       []apkDeltaOperation `json:"operations"`
}

type apkDeltaOperation struct {
	Action       APKAction    `json:"action"`
	Architecture Architecture `json:"architecture"`
	PackageName  string       `json:"package"`
	Source       string       `json:"source,omitempty"`
	SHA256       Digest       `json:"sha256,omitempty"`
}

func operationsFromDelta(data []byte, integer *ci.IntegerBatchManifest) ([]APKOperation, error) {
	delta, err := decodeAPKDelta(data)
	if err != nil {
		return nil, err
	}
	packages := make(map[string]ci.IntegerPublishedPackage, len(integer.Packages))
	for _, pkg := range integer.Packages {
		packages[string(pkg.Architecture)+"/"+pkg.Name] = pkg
	}
	operations := make([]APKOperation, 0, len(delta.Operations))
	seen := make(map[string]struct{}, len(delta.Operations))
	for operationIndex := range delta.Operations {
		operation := &delta.Operations[operationIndex]
		key := string(operation.Architecture) + "/" + operation.PackageName
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate APK delta operation %q", ErrComposeInvalid, key)
		}
		seen[key] = struct{}{}
		materialized, err := materializeDeltaOperation(operation, key, packages)
		if err != nil {
			return nil, err
		}
		operations = append(operations, materialized)
	}
	return operations, nil
}

func decodeAPKDelta(data []byte) (apkDeltaManifest, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return apkDeltaManifest{}, fmt.Errorf("%w: APK delta: %w", ErrComposeInvalid, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var delta apkDeltaManifest
	if err := decoder.Decode(&delta); err != nil {
		return apkDeltaManifest{}, fmt.Errorf("%w: decode APK delta: %w", ErrComposeInvalid, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apkDeltaManifest{}, fmt.Errorf("%w: trailing APK delta JSON", ErrComposeInvalid)
	}
	if delta.FormatVersion != 1 || !digestPattern.MatchString(string(delta.BaseSHA256)) ||
		!digestPattern.MatchString(string(delta.KeySHA256)) || delta.RepositoryFormat == "" {
		return apkDeltaManifest{}, fmt.Errorf("%w: malformed APK delta", ErrComposeInvalid)
	}
	return delta, nil
}

func materializeDeltaOperation(operation *apkDeltaOperation, key string, packages map[string]ci.IntegerPublishedPackage) (APKOperation, error) {
	switch operation.Action {
	case APKRemove:
		if operation.Source != "" || operation.SHA256 != "" {
			return APKOperation{}, fmt.Errorf("%w: remove %q carries source data", ErrComposeInvalid, key)
		}
		return APKOperation{Action: APKRemove, Architecture: operation.Architecture, PackageName: operation.PackageName}, nil
	case APKUpsert:
		pkg, exists := packages[key]
		if !exists || operation.Source == "" || operation.SHA256 != Digest(pkg.SHA256) {
			return APKOperation{}, fmt.Errorf("%w: upsert %q is not declared by Integer", ErrProducerConflict, key)
		}
		return APKOperation{
			Action: APKUpsert, Architecture: operation.Architecture, PackageName: operation.PackageName,
			ArtifactName: pkg.Artifact.Name, ArtifactDigest: Digest(pkg.Artifact.Digest),
		}, nil
	default:
		return APKOperation{}, fmt.Errorf("%w: unsupported APK delta action %q", ErrComposeInvalid, operation.Action)
	}
}
