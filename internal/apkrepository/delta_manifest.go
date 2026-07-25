package apkrepository

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	deltaManifestFormat = 1
	DeltaActionUpsert   = "upsert"
	DeltaActionRemove   = "remove"
)

var sha256Digest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type DeltaManifest struct {
	FormatVersion    int              `json:"format_version"`
	BaseSHA256       string           `json:"base_sha256"`
	RepositoryFormat string           `json:"repository_format"`
	KeySHA256        string           `json:"key_sha256"`
	Operations       []DeltaOperation `json:"operations"`
}

type DeltaOperation struct {
	Action       string `json:"action"`
	Architecture string `json:"architecture"`
	PackageName  string `json:"package"`
	Source       string `json:"source,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
}

func readDeltaManifest(path, packageDir string) (DeltaManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return DeltaManifest{}, fmt.Errorf("%w: open %q: %w", errInvalidDeltaManifest, path, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest DeltaManifest
	if err := decoder.Decode(&manifest); err != nil {
		return DeltaManifest{}, fmt.Errorf("%w: decode %q: %w", errInvalidDeltaManifest, path, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return DeltaManifest{}, fmt.Errorf("%w: %w", errInvalidDeltaManifest, err)
	}
	if err := validateDeltaManifest(&manifest); err != nil {
		return DeltaManifest{}, err
	}
	if err := validateDeclaredPackages(packageDir, manifest.Operations); err != nil {
		return DeltaManifest{}, err
	}
	return manifest, nil
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read trailing JSON: %w", err)
	}
	return fmt.Errorf("%w: multiple JSON values", errInvalidDeltaManifest)
}

func validateDeltaManifest(manifest *DeltaManifest) error {
	if manifest.FormatVersion != deltaManifestFormat {
		return fmt.Errorf("%w: format_version must be %d", errInvalidDeltaManifest, deltaManifestFormat)
	}
	if !sha256Digest.MatchString(manifest.BaseSHA256) {
		return fmt.Errorf("%w: invalid base_sha256", errInvalidDeltaManifest)
	}
	if !sha256Digest.MatchString(manifest.KeySHA256) {
		return fmt.Errorf("%w: invalid key_sha256", errInvalidDeltaManifest)
	}
	if manifest.RepositoryFormat == "" {
		return fmt.Errorf("%w: repository_format is required", errInvalidDeltaManifest)
	}
	if len(manifest.Operations) == 0 {
		return fmt.Errorf("%w: operations must not be empty", errInvalidDeltaManifest)
	}
	keys := make(map[packageKey]struct{}, len(manifest.Operations))
	sources := make(map[string]struct{})
	for index := range manifest.Operations {
		operation := &manifest.Operations[index]
		if err := validateDeltaOperation(operation); err != nil {
			return fmt.Errorf("operation %d: %w", index, err)
		}
		key := packageKey{architecture: operation.Architecture, name: operation.PackageName}
		if _, exists := keys[key]; exists {
			return fmt.Errorf("%w: %s/%s", errDuplicateDeltaMutation, key.architecture, key.name)
		}
		keys[key] = struct{}{}
		if operation.Source != "" {
			if _, exists := sources[operation.Source]; exists {
				return fmt.Errorf("%w: source %s", errDuplicateDeltaMutation, operation.Source)
			}
			sources[operation.Source] = struct{}{}
		}
	}
	return nil
}

func validateDeltaOperation(operation *DeltaOperation) error {
	if !isSupportedArchitecture(operation.Architecture) {
		return fmt.Errorf("%w: %s", errUnsupportedArchitecture, operation.Architecture)
	}
	if !safePackageName.MatchString(operation.PackageName) {
		return fmt.Errorf("%w: invalid package name %q", errInvalidDeltaManifest, operation.PackageName)
	}
	switch operation.Action {
	case DeltaActionUpsert:
		clean := filepath.ToSlash(filepath.Clean(operation.Source))
		if operation.Source == "" || filepath.IsAbs(operation.Source) || clean != operation.Source || clean == "." || strings.HasPrefix(clean, "../") || filepath.Ext(clean) != ".apk" {
			return fmt.Errorf("%w: unsafe upsert source %q", errInvalidDeltaManifest, operation.Source)
		}
		if !sha256Digest.MatchString(operation.SHA256) {
			return fmt.Errorf("%w: invalid upsert sha256", errInvalidDeltaManifest)
		}
	case DeltaActionRemove:
		if operation.Source != "" || operation.SHA256 != "" {
			return fmt.Errorf("%w: remove cannot declare source or sha256", errInvalidDeltaManifest)
		}
	default:
		return fmt.Errorf("%w: unsupported action %q", errInvalidDeltaManifest, operation.Action)
	}
	return nil
}

func validateDeclaredPackages(packageDir string, operations []DeltaOperation) error {
	declared := make(map[string]struct{})
	for _, operation := range operations {
		if operation.Source != "" {
			declared[operation.Source] = struct{}{}
		}
	}
	info, err := os.Stat(packageDir)
	if os.IsNotExist(err) && len(declared) == 0 {
		return nil
	}
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w: package directory %s", errDeltaPackageMissing, packageDir)
	}
	found := make(map[string]struct{})
	err = filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".apk" {
			return nil
		}
		relative, err := filepath.Rel(packageDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, exists := declared[relative]; !exists {
			return fmt.Errorf("%w: %s", errUndeclaredDeltaPackage, relative)
		}
		found[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	for source := range declared {
		if _, exists := found[source]; !exists {
			return fmt.Errorf("%w: %s", errDeltaPackageMissing, source)
		}
	}
	return nil
}
