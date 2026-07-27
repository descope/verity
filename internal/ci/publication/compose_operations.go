package publication

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

func ParseAPKOperationsCanonical(data []byte) ([]APKOperation, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return nil, fmt.Errorf("%w: APK operations: %w", ErrComposeInvalid, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var operations []APKOperation
	if err := decoder.Decode(&operations); err != nil {
		return nil, fmt.Errorf("%w: decode APK operations: %w", ErrComposeInvalid, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing APK operations JSON", ErrComposeInvalid)
	}
	canonical := append([]APKOperation(nil), operations...)
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Architecture != canonical[j].Architecture {
			return canonical[i].Architecture < canonical[j].Architecture
		}
		if canonical[i].PackageName != canonical[j].PackageName {
			return canonical[i].PackageName < canonical[j].PackageName
		}
		return canonical[i].Action < canonical[j].Action
	})
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal APK operations: %w", err)
	}
	if !bytes.Equal(encoded, data) {
		return nil, ErrNonCanonicalManifest
	}
	return operations, nil
}
