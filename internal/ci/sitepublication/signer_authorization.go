package sitepublication

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
)

func buildSignerAuthorization(plan *PublicationPlan, spec *SignerExecutionSpec) (SignerInputAuthorization, publication.Digest, error) {
	manifest, manifestDigest, err := readSignerManifest(spec.WorkspaceDir, spec.ManifestPath)
	if err != nil {
		return SignerInputAuthorization{}, "", err
	}
	if manifestDigest != plan.ManifestDigest || manifest.Mode != plan.Mode || manifest.SignerDigest != plan.SignerDigest {
		return SignerInputAuthorization{}, "", fmt.Errorf("%w: publication manifest does not match plan", ErrInvalidSignerPlan)
	}
	authorization := SignerInputAuthorization{
		SchemaVersion: SchemaVersion, PublicationPlanDigest: plan.PlanDigest, ManifestDigest: manifestDigest,
		Mode: plan.Mode, ManifestPath: spec.ManifestPath, PackagesPath: spec.PackagesPath,
		BaseAPKPath: spec.BaseAPKPath, DeltaManifestPath: spec.DeltaManifestPath, PublicKeyPath: spec.PublicKeyPath,
		APKOperations: append([]publication.APKOperation(nil), manifest.APKOperations...),
	}
	for _, relative := range []string{spec.ManifestPath, spec.PublicKeyPath} {
		input, inputErr := authorizeSignerFile(spec.WorkspaceDir, relative, false)
		if inputErr != nil {
			return SignerInputAuthorization{}, "", inputErr
		}
		authorization.Inputs = append(authorization.Inputs, input)
	}
	packages, err := authorizeSignerDirectory(spec.WorkspaceDir, spec.PackagesPath, func(string) bool { return true })
	if err != nil {
		return SignerInputAuthorization{}, "", err
	}
	authorization.Inputs = append(authorization.Inputs, packages...)
	authorization.Packages = append(authorization.Packages, packages...)
	if len(authorization.Packages) == 0 {
		return SignerInputAuthorization{}, "", fmt.Errorf("%w: no authorized packages", ErrInvalidSignerPlan)
	}
	if spec.Mode == publication.ModeDelta {
		delta, inputErr := authorizeSignerFile(spec.WorkspaceDir, spec.DeltaManifestPath, false)
		if inputErr != nil {
			return SignerInputAuthorization{}, "", inputErr
		}
		authorization.Inputs = append(authorization.Inputs, delta)
		baseInputs, scanErr := authorizeSignerDirectory(
			spec.WorkspaceDir,
			spec.BaseAPKPath,
			func(relative string) bool { return strings.HasSuffix(relative, ".apk") },
		)
		if scanErr != nil {
			return SignerInputAuthorization{}, "", scanErr
		}
		authorization.Inputs = append(authorization.Inputs, baseInputs...)
	}
	encoded, err := marshalSignerAuthorizationCanonical(&authorization)
	if err != nil {
		return SignerInputAuthorization{}, "", err
	}
	sum := sha256.Sum256(encoded)
	return authorization, publication.Digest("sha256:" + hex.EncodeToString(sum[:])), nil
}

func readSignerManifest(root, relative string) (publication.Manifest, publication.Digest, error) {
	manifestBytes, err := os.ReadFile(signerHostPath(root, relative))
	if err != nil {
		return publication.Manifest{}, "", fmt.Errorf("%w: read publication manifest: %w", ErrInvalidSignerPlan, err)
	}
	manifest, err := publication.ParseCanonical(manifestBytes)
	if err != nil {
		return publication.Manifest{}, "", fmt.Errorf("%w: parse publication manifest: %w", ErrInvalidSignerPlan, err)
	}
	manifestDigest, err := publication.DigestManifest(&manifest)
	if err != nil {
		return publication.Manifest{}, "", fmt.Errorf("%w: digest publication manifest: %w", ErrInvalidSignerPlan, err)
	}
	return manifest, manifestDigest, nil
}

func authorizeSignerFile(root, relative string, semantic bool) (SignerAuthorizedInput, error) {
	path := signerHostPath(root, relative)
	if _, err := validateSignerRegularPath(relative, path); err != nil {
		return SignerAuthorizedInput{}, err
	}
	digest, err := signerStableFileDigest(path)
	if err != nil {
		return SignerAuthorizedInput{}, fmt.Errorf("%w: hash %s: %w", ErrInvalidSignerPlan, relative, err)
	}
	input := SignerAuthorizedInput{Path: relative, ContentDigest: digest}
	if semantic {
		semanticDigest, semanticErr := signerPackageSemanticDigest(path)
		if semanticErr != nil {
			return SignerAuthorizedInput{}, fmt.Errorf("%w: inspect package %s: %w", ErrInvalidSignerPlan, relative, semanticErr)
		}
		input.SemanticDigest = semanticDigest
	}
	return input, nil
}

func authorizeSignerDirectory(
	root string,
	relativeRoot string,
	semanticFor func(string) bool,
) ([]SignerAuthorizedInput, error) {
	records, err := scanSignerDirectory(signerHostPath(root, relativeRoot))
	if err != nil {
		return nil, err
	}
	inputs := make([]SignerAuthorizedInput, 0, len(records))
	for _, record := range records {
		relative := filepath.ToSlash(filepath.Join(relativeRoot, filepath.FromSlash(record.relative)))
		input, inputErr := authorizeSignerFile(root, relative, semanticFor(relative))
		if inputErr != nil {
			return nil, inputErr
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func marshalSignerAuthorizationCanonical(authorization *SignerInputAuthorization) ([]byte, error) {
	if err := validateSignerAuthorization(authorization); err != nil {
		return nil, err
	}
	canonical := *authorization
	canonical.APKOperations = append([]publication.APKOperation(nil), authorization.APKOperations...)
	canonical.Inputs = append([]SignerAuthorizedInput(nil), authorization.Inputs...)
	canonical.Packages = append([]SignerAuthorizedInput(nil), authorization.Packages...)
	sort.Slice(canonical.APKOperations, func(i, j int) bool {
		left, right := canonical.APKOperations[i], canonical.APKOperations[j]
		if left.Architecture != right.Architecture {
			return left.Architecture < right.Architecture
		}
		if left.PackageName != right.PackageName {
			return left.PackageName < right.PackageName
		}
		return left.Action < right.Action
	})
	sort.Slice(canonical.Inputs, func(i, j int) bool { return canonical.Inputs[i].Path < canonical.Inputs[j].Path })
	sort.Slice(canonical.Packages, func(i, j int) bool { return canonical.Packages[i].Path < canonical.Packages[j].Path })
	return json.Marshal(&canonical)
}

func parseSignerAuthorizationBase64(value string) (SignerInputAuthorization, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return SignerInputAuthorization{}, fmt.Errorf("%w: decode input authorization: %w", ErrInvalidSignerPlan, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var authorization SignerInputAuthorization
	if err := decoder.Decode(&authorization); err != nil {
		return SignerInputAuthorization{}, fmt.Errorf("%w: decode input authorization: %w", ErrInvalidSignerPlan, err)
	}
	canonical, err := marshalSignerAuthorizationCanonical(&authorization)
	if err != nil {
		return SignerInputAuthorization{}, err
	}
	if !bytes.Equal(data, canonical) {
		return SignerInputAuthorization{}, fmt.Errorf("%w: non-canonical input authorization", ErrInvalidSignerPlan)
	}
	return authorization, nil
}

func signerAuthorizationBase64(authorization *SignerInputAuthorization) (string, error) {
	data, err := marshalSignerAuthorizationCanonical(authorization)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func VerifySignerInputsBase64(root, value string, inputDigest, planDigest, manifestDigest publication.Digest) (SignerInputAuthorization, error) {
	authorization, err := parseSignerAuthorizationBase64(value)
	if err != nil {
		return SignerInputAuthorization{}, err
	}
	if err := VerifySignerInputs(root, &authorization, inputDigest, planDigest, manifestDigest); err != nil {
		return SignerInputAuthorization{}, err
	}
	return authorization, nil
}

func validateSignerAuthorization(authorization *SignerInputAuthorization) error {
	if authorization == nil || authorization.SchemaVersion != SchemaVersion ||
		!digestPattern.MatchString(string(authorization.PublicationPlanDigest)) ||
		!digestPattern.MatchString(string(authorization.ManifestDigest)) || authorization.Inputs == nil || authorization.Packages == nil {
		return fmt.Errorf("%w: invalid input authorization", ErrInvalidSignerPlan)
	}
	seen := make(map[string]struct{}, len(authorization.Inputs))
	for _, input := range authorization.Inputs {
		if _, err := cleanSignerPath(input.Path); err != nil || !digestPattern.MatchString(string(input.ContentDigest)) {
			return fmt.Errorf("%w: invalid authorized input %q", ErrInvalidSignerPlan, input.Path)
		}
		if input.SemanticDigest != "" && !digestPattern.MatchString(string(input.SemanticDigest)) {
			return fmt.Errorf("%w: invalid semantic digest for %q", ErrInvalidSignerPlan, input.Path)
		}
		if _, exists := seen[input.Path]; exists {
			return fmt.Errorf("%w: duplicate authorized input %q", ErrInvalidSignerPlan, input.Path)
		}
		seen[input.Path] = struct{}{}
	}
	return nil
}

func VerifySignerInputs(root string, authorization *SignerInputAuthorization, inputDigest, planDigest, manifestDigest publication.Digest) error {
	encoded, err := marshalSignerAuthorizationCanonical(authorization)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	actualDigest := publication.Digest("sha256:" + hex.EncodeToString(sum[:]))
	if inputDigest != actualDigest || authorization.PublicationPlanDigest != planDigest || authorization.ManifestDigest != manifestDigest {
		return fmt.Errorf("%w: signer input binding mismatch", ErrInvalidSignerPlan)
	}
	manifest, actualManifestDigest, err := readSignerManifest(root, authorization.ManifestPath)
	if err != nil {
		return err
	}
	if actualManifestDigest != manifestDigest || manifest.Mode != authorization.Mode ||
		!slices.Equal(manifest.APKOperations, authorization.APKOperations) {
		return fmt.Errorf("%w: bound manifest content mismatch", ErrInvalidSignerPlan)
	}
	for _, input := range authorization.Inputs {
		actual, inputErr := authorizeSignerFile(root, input.Path, input.SemanticDigest != "")
		if inputErr != nil || actual != input {
			return fmt.Errorf("%w: authorized input changed: %s", ErrInvalidSignerPlan, input.Path)
		}
	}
	packageRecords, err := scanSignerDirectory(signerHostPath(root, authorization.PackagesPath))
	if err != nil || len(packageRecords) != len(authorization.Packages) {
		return fmt.Errorf("%w: authorized package set changed", ErrInvalidSignerPlan)
	}
	return nil
}
