package sitepublication

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type canonicalJSONCodec[T any] struct {
	label    string
	invalid  error
	validate func(*T) error
}

func (codec canonicalJSONCodec[T]) marshal(value *T) ([]byte, error) {
	if err := codec.validate(value); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", codec.label, err)
	}
	return data, nil
}

func (codec canonicalJSONCodec[T]) parse(data []byte) (T, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value T
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("%w: decode %s: %w", codec.invalid, codec.label, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return value, fmt.Errorf("%w: trailing %s JSON", codec.invalid, codec.label)
	}
	canonical, err := codec.marshal(&value)
	if err != nil {
		return value, err
	}
	if !bytes.Equal(data, canonical) {
		return value, fmt.Errorf("%w: non-canonical %s JSON", codec.invalid, codec.label)
	}
	return value, nil
}
