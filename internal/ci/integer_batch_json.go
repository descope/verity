package ci

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

var (
	errIntegerTrailingJSON  = errors.New("trailing JSON value")
	errIntegerJSONObjectKey = errors.New("JSON object key is not a string")
	errIntegerDuplicateKey  = errors.New("duplicate JSON key")
	errIntegerJSONDelimiter = errors.New("unexpected JSON delimiter")
)

func ParseIntegerBatchPlan(data []byte) (IntegerBatchPlan, error) {
	var plan IntegerBatchPlan
	if err := decodeIntegerJSON(data, &plan); err != nil {
		return IntegerBatchPlan{}, fmt.Errorf("%w: %w", ErrIntegerBatchPlan, err)
	}
	if err := validateIntegerBatchPlan(&plan); err != nil {
		return IntegerBatchPlan{}, err
	}
	return plan, nil
}

func MarshalIntegerBatchPlan(plan *IntegerBatchPlan) ([]byte, error) {
	if err := validateIntegerBatchPlan(plan); err != nil {
		return nil, err
	}
	canonical := *plan
	canonical.Targets = append([]IntegerBatchTarget(nil), plan.Targets...)
	canonical.Packages = append([]IntegerPlannedPackage(nil), plan.Packages...)
	sort.Slice(canonical.Targets, func(i, j int) bool { return canonical.Targets[i].ID() < canonical.Targets[j].ID() })
	sortIntegerPlannedPackages(canonical.Packages)
	return marshalIntegerJSON(&canonical)
}

func ParseIntegerComponentManifest(data []byte) (IntegerComponentManifest, error) {
	var manifest IntegerComponentManifest
	if err := decodeIntegerJSON(data, &manifest); err != nil {
		return IntegerComponentManifest{}, fmt.Errorf("parse Integer component manifest: %w", err)
	}
	if err := validateIntegerComponentManifest(&manifest); err != nil {
		return IntegerComponentManifest{}, err
	}
	return manifest, nil
}

func MarshalIntegerComponentManifest(manifest *IntegerComponentManifest) ([]byte, error) {
	if manifest == nil {
		return nil, fmt.Errorf("%w: component manifest is required", ErrIntegerBatchPlan)
	}
	if err := validateIntegerComponentManifest(manifest); err != nil {
		return nil, err
	}
	canonical := *manifest
	canonical.Packages = append([]IntegerPackageFile(nil), manifest.Packages...)
	sortIntegerPackageFiles(canonical.Packages)
	return marshalIntegerJSON(&canonical)
}

func ParseIntegerShardInventory(data []byte) (IntegerShardInventory, error) {
	var inventory IntegerShardInventory
	if err := decodeIntegerJSON(data, &inventory); err != nil {
		return IntegerShardInventory{}, fmt.Errorf("parse Integer shard inventory: %w", err)
	}
	if err := validateIntegerShardInventory(&inventory); err != nil {
		return IntegerShardInventory{}, err
	}
	return inventory, nil
}

func MarshalIntegerShardInventory(inventory *IntegerShardInventory) ([]byte, error) {
	if inventory == nil {
		return nil, fmt.Errorf("%w: shard inventory is required", ErrIntegerBatchPlan)
	}
	if err := validateIntegerShardInventory(inventory); err != nil {
		return nil, err
	}
	canonical := *inventory
	canonical.Packages = append([]IntegerPackageFile(nil), inventory.Packages...)
	sortIntegerPackageFiles(canonical.Packages)
	return marshalIntegerJSON(&canonical)
}

func ParseIntegerShardManifest(data []byte) (IntegerShardManifest, error) {
	var manifest IntegerShardManifest
	if err := decodeIntegerJSON(data, &manifest); err != nil {
		return IntegerShardManifest{}, fmt.Errorf("parse Integer shard manifest: %w", err)
	}
	if err := validateIntegerShardManifest(&manifest); err != nil {
		return IntegerShardManifest{}, err
	}
	return manifest, nil
}

func MarshalIntegerShardManifest(manifest *IntegerShardManifest) ([]byte, error) {
	if manifest == nil {
		return nil, fmt.Errorf("%w: shard manifest is required", ErrIntegerBatchPlan)
	}
	if err := validateIntegerShardManifest(manifest); err != nil {
		return nil, err
	}
	canonical := *manifest
	canonical.Packages = append([]IntegerPackageFile(nil), manifest.Packages...)
	sortIntegerPackageFiles(canonical.Packages)
	return marshalIntegerJSON(&canonical)
}

func ParseIntegerBatchManifest(data []byte) (IntegerBatchManifest, error) {
	var manifest IntegerBatchManifest
	if err := decodeIntegerJSON(data, &manifest); err != nil {
		return IntegerBatchManifest{}, fmt.Errorf("parse Integer batch manifest: %w", err)
	}
	if err := validateIntegerBatchManifest(&manifest); err != nil {
		return IntegerBatchManifest{}, err
	}
	return manifest, nil
}

func MarshalIntegerBatchManifest(manifest *IntegerBatchManifest) ([]byte, error) {
	if manifest == nil {
		return nil, fmt.Errorf("%w: batch manifest is required", ErrIntegerBatchPlan)
	}
	if err := validateIntegerBatchManifest(manifest); err != nil {
		return nil, err
	}
	canonical := *manifest
	canonical.Shards = append([]IntegerShardManifest(nil), manifest.Shards...)
	canonical.Packages = append([]IntegerPublishedPackage(nil), manifest.Packages...)
	sort.Slice(canonical.Shards, func(i, j int) bool { return canonical.Shards[i].Shard < canonical.Shards[j].Shard })
	sort.Slice(canonical.Packages, func(i, j int) bool {
		if canonical.Packages[i].Architecture != canonical.Packages[j].Architecture {
			return canonical.Packages[i].Architecture < canonical.Packages[j].Architecture
		}
		return canonical.Packages[i].Name < canonical.Packages[j].Name
	})
	return marshalIntegerJSON(&canonical)
}

func decodeIntegerJSON(data []byte, destination any) error {
	if err := rejectDuplicateIntegerJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errIntegerTrailingJSON
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func rejectDuplicateIntegerJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	return walkIntegerJSON(decoder, false)
}

func walkIntegerJSON(decoder *json.Decoder, insideObject bool) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON token: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("read JSON key: %w", err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return errIntegerJSONObjectKey
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("%w %q", errIntegerDuplicateKey, key)
			}
			keys[key] = struct{}{}
			if err := walkIntegerJSON(decoder, true); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkIntegerJSON(decoder, false); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		if insideObject {
			return fmt.Errorf("%w %q", errIntegerJSONDelimiter, delimiter)
		}
		return nil
	}
}

func marshalIntegerJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal Integer JSON: %w", err)
	}
	return data, nil
}

func sortIntegerPlannedPackages(packages []IntegerPlannedPackage) {
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Architecture != packages[j].Architecture {
			return packages[i].Architecture < packages[j].Architecture
		}
		return packages[i].Name < packages[j].Name
	})
}

func sortIntegerPackageFiles(packages []IntegerPackageFile) {
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Architecture != packages[j].Architecture {
			return packages[i].Architecture < packages[j].Architecture
		}
		return packages[i].Name < packages[j].Name
	})
}
