package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

var (
	ErrDuplicateKey  = errors.New("duplicate JSON key")
	ErrDelimiter     = errors.New("unexpected JSON delimiter")
	ErrTrailingValue = errors.New("trailing JSON value")
	ErrUnknownKey    = errors.New("unknown JSON key")
)

func Decode[T any](content []byte, destination *T) error {
	if destination == nil {
		return fmt.Errorf("decode strict JSON: %w", ErrInvalidDestination)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := validateValue(decoder, reflect.TypeOf(*destination), "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return ErrTrailingValue
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	if err := json.Unmarshal(content, destination); err != nil {
		return fmt.Errorf("decode JSON value: %w", err)
	}
	return nil
}

var ErrInvalidDestination = errors.New("invalid JSON destination")

func validateValue(decoder *json.Decoder, destination reflect.Type, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode JSON token at %s: %w", path, err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	destination = indirect(destination)
	switch delimiter {
	case '{':
		return validateObject(decoder, destination, path)
	case '[':
		return validateArray(decoder, destination, path)
	default:
		return fmt.Errorf("%w %q at %s", ErrDelimiter, delimiter, path)
	}
}

func validateObject(decoder *json.Decoder, destination reflect.Type, path string) error {
	fields := jsonFields(destination)
	seen := make(map[string]string)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode JSON key at %s: %w", path, err)
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("decode JSON key at %s: %w", path, ErrUnknownKey)
		}
		folded := strings.ToLower(key)
		if previous, exists := seen[folded]; exists {
			return fmt.Errorf("%w at %s: %q conflicts with %q", ErrDuplicateKey, path, key, previous)
		}
		seen[folded] = key
		fieldType, exists := fields[key]
		if destination.Kind() != reflect.Map && !exists {
			return fmt.Errorf("%w at %s: %q", ErrUnknownKey, path, key)
		}
		if destination.Kind() == reflect.Map {
			fieldType = destination.Elem()
		}
		if err := validateValue(decoder, fieldType, path+"."+key); err != nil {
			return err
		}
	}
	return consumeDelimiter(decoder, '}', path)
}

func validateArray(decoder *json.Decoder, destination reflect.Type, path string) error {
	element := destination
	if destination.Kind() == reflect.Array || destination.Kind() == reflect.Slice {
		element = destination.Elem()
	}
	for index := 0; decoder.More(); index++ {
		if err := validateValue(decoder, element, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return consumeDelimiter(decoder, ']', path)
}

func consumeDelimiter(decoder *json.Decoder, expected json.Delim, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode JSON delimiter at %s: %w", path, err)
	}
	if token != expected {
		return fmt.Errorf("%w at %s: expected %q", ErrDelimiter, path, expected)
	}
	return nil
}

func indirect(value reflect.Type) reflect.Type {
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

func jsonFields(value reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	if value.Kind() != reflect.Struct {
		return fields
	}
	for field := range value.Fields() {
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}
