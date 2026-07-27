package publication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

func MarshalCanonical(manifest *Manifest) ([]byte, error) {
	if manifest == nil {
		return nil, invalidManifest("manifest is required")
	}
	if err := validateManifestShape(manifest); err != nil {
		return nil, err
	}
	canonical := *manifest
	canonical.Components = append([]Component(nil), manifest.Components...)
	canonical.APKOperations = append([]APKOperation(nil), manifest.APKOperations...)
	sort.Slice(canonical.Components, func(i, j int) bool {
		return compareComponent(&canonical.Components[i], &canonical.Components[j]) < 0
	})
	sort.Slice(canonical.APKOperations, func(i, j int) bool {
		left := &canonical.APKOperations[i]
		right := &canonical.APKOperations[j]
		if left.Architecture != right.Architecture {
			return left.Architecture < right.Architecture
		}
		if left.PackageName != right.PackageName {
			return left.PackageName < right.PackageName
		}
		return left.Action < right.Action
	})
	data, err := json.Marshal(canonicalManifestValue(&canonical))
	if err != nil {
		return nil, fmt.Errorf("marshal publication manifest: %w", err)
	}
	return data, nil
}

type canonicalManifest struct {
	SchemaVersion                 int            `json:"schema_version"`
	SourceSHA                     SourceSHA      `json:"source_sha"`
	RunID                         RunID          `json:"run_id"`
	RunAttempt                    RunAttempt     `json:"run_attempt"`
	BatchID                       BatchID        `json:"batch_id"`
	Mode                          Mode           `json:"mode"`
	PreviousManifestDigest        Digest         `json:"previous_manifest_digest,omitempty"`
	Components                    []Component    `json:"components"`
	SignerDigest                  Digest         `json:"signer_digest"`
	SigningKeyEpoch               *uint64        `json:"signing_key_epoch,omitempty"`
	ActiveSigningKeyFingerprint   *string        `json:"active_signing_key_fingerprint,omitempty"`
	TrustedSigningKeyFingerprints *[]string      `json:"trusted_signing_key_fingerprints,omitempty"`
	RevokedSigningKeyFingerprints *[]string      `json:"revoked_signing_key_fingerprints,omitempty"`
	APKOperations                 []APKOperation `json:"apk_operations"`
}

func canonicalManifestValue(manifest *Manifest) canonicalManifest {
	canonical := canonicalManifest{
		SchemaVersion:          manifest.SchemaVersion,
		SourceSHA:              manifest.SourceSHA,
		RunID:                  manifest.RunID,
		RunAttempt:             manifest.RunAttempt,
		BatchID:                manifest.BatchID,
		Mode:                   manifest.Mode,
		PreviousManifestDigest: manifest.PreviousManifestDigest,
		Components:             manifest.Components,
		SignerDigest:           manifest.SignerDigest,
		APKOperations:          manifest.APKOperations,
	}
	if !signingKeyStatePresent(manifest) {
		return canonical
	}
	epoch := manifest.SigningKeyEpoch
	active := manifest.ActiveSigningKeyFingerprint
	trusted := append([]string(nil), manifest.TrustedSigningKeyFingerprints...)
	revoked := append([]string(nil), manifest.RevokedSigningKeyFingerprints...)
	if manifest.TrustedSigningKeyFingerprints != nil && trusted == nil {
		trusted = []string{}
	}
	if manifest.RevokedSigningKeyFingerprints != nil && revoked == nil {
		revoked = []string{}
	}
	canonical.SigningKeyEpoch = &epoch
	canonical.ActiveSigningKeyFingerprint = &active
	canonical.TrustedSigningKeyFingerprints = &trusted
	canonical.RevokedSigningKeyFingerprints = &revoked
	return canonical
}

func DigestManifest(manifest *Manifest) (Digest, error) {
	data, err := MarshalCanonical(manifest)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return Digest("sha256:" + hex.EncodeToString(sum[:])), nil
}

func ParseCanonical(data []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: decode JSON: %w", ErrInvalidManifest, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidManifest)
		}
		return Manifest{}, fmt.Errorf("%w: decode trailing JSON: %w", ErrInvalidManifest, err)
	}
	if err := validateSigningKeyFieldPresence(data); err != nil {
		return Manifest{}, err
	}
	canonical, err := MarshalCanonical(&manifest)
	if err != nil {
		return Manifest{}, err
	}
	if !bytes.Equal(data, canonical) {
		return Manifest{}, ErrNonCanonicalManifest
	}
	return manifest, nil
}

func validateSigningKeyFieldPresence(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("%w: inspect signing key fields: %w", ErrInvalidManifest, err)
	}
	fieldNames := []string{
		"signing_key_epoch",
		"active_signing_key_fingerprint",
		"trusted_signing_key_fingerprints",
		"revoked_signing_key_fingerprints",
	}
	present := 0
	for _, fieldName := range fieldNames {
		if _, ok := fields[fieldName]; ok {
			present++
		}
	}
	if present != 0 && present != len(fieldNames) {
		return invalidManifest("signing key state fields must be present together")
	}
	if present == len(fieldNames) && bytes.Equal(bytes.TrimSpace(fields["signing_key_epoch"]), []byte("null")) {
		return invalidManifest("signing key epoch must be an integer")
	}
	return nil
}

func MarshalComponentsCanonical(components []Component) ([]byte, error) {
	if err := validateComponents(components); err != nil {
		return nil, err
	}
	canonical := append([]Component(nil), components...)
	sort.Slice(canonical, func(i, j int) bool {
		return compareComponent(&canonical[i], &canonical[j]) < 0
	})
	data, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal publication components: %w", err)
	}
	return data, nil
}

func ParseComponentsCanonical(data []byte) ([]Component, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var components []Component
	if err := decoder.Decode(&components); err != nil {
		return nil, fmt.Errorf("%w: decode components JSON: %w", ErrInvalidManifest, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing components JSON", ErrInvalidManifest)
	}
	canonical, err := MarshalComponentsCanonical(components)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(data, canonical) {
		return nil, ErrNonCanonicalManifest
	}
	return components, nil
}
