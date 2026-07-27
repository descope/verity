// Package signerlock parses and validates the pinned APK repository signer.
package signerlock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	SignerImageRepository   = "ghcr.io/verity-org/apk-repository-signer"
	TrustedWorkflowIdentity = "github.com/verity-org/verity/.github/workflows/integer-build-image-reusable.yaml"
	digestPrefix            = "sha256:"
	digestHexLength         = 64
	sourceSHAHexLength      = 40
)

var (
	ErrMalformed        = errors.New("malformed signer lock")
	ErrDuplicateField   = errors.New("duplicate signer lock field")
	ErrInvalidFieldName = errors.New("non-canonical signer lock field name")
	ErrMultipleValues   = errors.New("multiple JSON values")
	ErrJSONStructure    = errors.New("invalid JSON structure")
	ErrRequiredField    = errors.New("required signer lock field is missing")
	ErrInvalidImage     = errors.New("invalid signer image")
	ErrInvalidDigest    = errors.New("invalid signer image digest")
	ErrInvalidWorkflow  = errors.New("untrusted signer workflow identity")
	ErrInvalidSourceSHA = errors.New("invalid signer source SHA")
	ErrStaleSource      = errors.New("signer source SHA does not match the expected source")
	ErrBootstrap        = errors.New("bootstrap signer lock is not runnable")
	ErrNotRunnable      = errors.New("signer lock is not runnable")
)

// Lock is the trusted, digest-pinned signer image contract.
type Lock struct {
	Image     string `json:"image"`
	Digest    string `json:"digest"`
	Workflow  string `json:"workflow"`
	SourceSHA string `json:"source_sha"`
	Bootstrap bool   `json:"bootstrap,omitempty"`
	Runnable  bool   `json:"runnable"`
}

// ValidationError identifies the field that made a signer lock unusable.
type ValidationError struct {
	Field string
	Value string
	Cause error
}

func (e *ValidationError) Error() string {
	if e.Value == "" {
		return fmt.Sprintf("signer lock field %q: %v", e.Field, e.Cause)
	}
	return fmt.Sprintf("signer lock field %q value %q: %v", e.Field, e.Value, e.Cause)
}

func (e *ValidationError) Unwrap() error { return e.Cause }

type lockDocument struct {
	Image     *string `json:"image"`
	Digest    *string `json:"digest"`
	Workflow  *string `json:"workflow"`
	SourceSHA *string `json:"source_sha"`
	Bootstrap *bool   `json:"bootstrap"`
	Runnable  *bool   `json:"runnable"`
}

// Parse decodes one strict JSON signer lock and validates its trust contract.
func Parse(data []byte) (Lock, error) {
	if err := rejectDuplicateMemberNames(data); err != nil {
		return Lock{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var document lockDocument
	if err := decoder.Decode(&document); err != nil {
		return Lock{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Lock{}, fmt.Errorf("%w: multiple JSON values", ErrMalformed)
		}
		return Lock{}, fmt.Errorf("%w: trailing JSON: %w", ErrMalformed, err)
	}

	lock, err := document.lock()
	if err != nil {
		return Lock{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if err := Validate(lock); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func rejectDuplicateMemberNames(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ErrMultipleValues
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	return scanJSONValueToken(decoder, token)
}

func scanJSONValueToken(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return scanJSONObject(decoder)
	case '[':
		return scanJSONArray(decoder)
	default:
		return fmt.Errorf("%w: unexpected delimiter %q", ErrJSONStructure, delimiter)
	}
}

func scanJSONObject(decoder *json.Decoder) error {
	seenNames := make(map[string]struct{})
	seenFields := make(map[string]string)
	var alias string
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter == '}' {
				if alias != "" {
					return fmt.Errorf("%w: %q", ErrInvalidFieldName, alias)
				}
				return nil
			}
			return fmt.Errorf("%w: unexpected delimiter %q in object", ErrJSONStructure, delimiter)
		}

		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("%w: object member name has type %T", ErrJSONStructure, token)
		}
		if _, ok := seenNames[key]; ok {
			return fmt.Errorf("%w: %q", ErrDuplicateField, key)
		}
		seenNames[key] = struct{}{}
		if canonical, ok := canonicalLockFieldName(key); ok {
			if previous, ok := seenFields[canonical]; ok {
				return fmt.Errorf("%w: %q and %q", ErrDuplicateField, previous, key)
			}
			seenFields[canonical] = key
			if key != canonical && alias == "" {
				alias = key
			}
		}
		if err := scanJSONValue(decoder); err != nil {
			return err
		}
	}
}

func canonicalLockFieldName(key string) (string, bool) {
	for _, canonical := range []string{"image", "digest", "workflow", "source_sha", "bootstrap", "runnable"} {
		if strings.EqualFold(key, canonical) {
			return canonical, true
		}
	}
	return "", false
}

func scanJSONArray(decoder *json.Decoder) error {
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok && delimiter == ']' {
			return nil
		}
		if err := scanJSONValueToken(decoder, token); err != nil {
			return err
		}
	}
}

// Load reads, parses, and validates a signer lock file.
func Load(path string) (Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Lock{}, fmt.Errorf("read signer lock %q: %w", path, err)
	}
	lock, err := Parse(data)
	if err != nil {
		return Lock{}, fmt.Errorf("parse signer lock %q: %w", path, err)
	}
	return lock, nil
}

func (d lockDocument) lock() (Lock, error) {
	fields := []struct {
		name  string
		value *string
	}{{"image", d.Image}, {"digest", d.Digest}, {"workflow", d.Workflow}, {"source_sha", d.SourceSHA}}
	for _, field := range fields {
		if field.value == nil {
			return Lock{}, &ValidationError{Field: field.name, Cause: ErrRequiredField}
		}
	}
	if d.Runnable == nil {
		return Lock{}, &ValidationError{Field: "runnable", Cause: ErrRequiredField}
	}

	return Lock{
		Image:     *d.Image,
		Digest:    *d.Digest,
		Workflow:  *d.Workflow,
		SourceSHA: *d.SourceSHA,
		Bootstrap: d.Bootstrap != nil && *d.Bootstrap,
		Runnable:  *d.Runnable,
	}, nil
}
