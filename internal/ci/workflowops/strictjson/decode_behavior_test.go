package strictjson

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type strictJSONNested struct {
	Value string `json:"value"`
}

type strictJSONFixture struct {
	Name    string                       `json:"name"`
	Nested  *strictJSONNested            `json:"nested"`
	Items   []strictJSONNested           `json:"items"`
	Labels  map[string]*strictJSONNested `json:"labels"`
	Default string
	Ignored string `json:"-"`
}

func TestDecode_decodes_nested_structures_maps_arrays_and_default_field_names(t *testing.T) {
	// Given
	content := []byte(`{
		"name":"sentinel",
		"nested":{"value":"pointer"},
		"items":[{"value":"array"}],
		"labels":{"stable":{"value":"map"}},
		"Default":"default-name"
	}`)
	var destination strictJSONFixture

	// When
	err := Decode(content, &destination)

	// Then
	require.NoError(t, err)
	require.Equal(t, "sentinel", destination.Name)
	require.Equal(t, "pointer", destination.Nested.Value)
	require.Equal(t, []strictJSONNested{{Value: "array"}}, destination.Items)
	require.Equal(t, "map", destination.Labels["stable"].Value)
	require.Equal(t, "default-name", destination.Default)
}

func TestDecode_rejects_ambiguous_unknown_trailing_and_malformed_JSON(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     error
		wantMessage string
	}{
		{name: "unknown root key", content: `{"unknown":true}`, wantErr: ErrUnknownKey},
		{name: "unknown nested key", content: `{"nested":{"extra":true}}`, wantErr: ErrUnknownKey},
		{name: "case folded duplicate", content: `{"name":"first","NAME":"second"}`, wantErr: ErrDuplicateKey},
		{name: "duplicate map key", content: `{"labels":{"Stable":{"value":"one"},"stable":{"value":"two"}}}`, wantErr: ErrDuplicateKey},
		{name: "trailing value", content: `{"name":"sentinel"} []`, wantErr: ErrTrailingValue},
		{name: "malformed trailing value", content: `{"name":"sentinel"} x`, wantMessage: "decode trailing JSON"},
		{name: "empty document", content: ``, wantMessage: "decode JSON token at $"},
		{name: "truncated object", content: `{"name"`, wantMessage: "decode JSON token at $.name"},
		{name: "truncated array", content: `{"items":[`, wantMessage: "decode JSON delimiter at $.items"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			var destination strictJSONFixture

			// When
			err := Decode([]byte(test.content), &destination)

			// Then
			require.Error(t, err)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			}
			if test.wantMessage != "" {
				require.ErrorContains(t, err, test.wantMessage)
			}
		})
	}
}

func TestDecode_rejects_invalid_destination_and_type_mismatch(t *testing.T) {
	t.Run("nil destination", func(t *testing.T) {
		// Given
		var destination *strictJSONFixture

		// When
		err := Decode([]byte(`{}`), destination)

		// Then
		require.ErrorIs(t, err, ErrInvalidDestination)
	})

	t.Run("structurally valid value with incompatible type", func(t *testing.T) {
		// Given
		var destination int

		// When
		err := Decode([]byte(`[1]`), &destination)

		// Then
		require.ErrorContains(t, err, "decode JSON value")
	})
}

func TestDecode_delimiter_helpers_fail_closed_on_unexpected_or_missing_closers(t *testing.T) {
	t.Run("unexpected value delimiter", func(t *testing.T) {
		// Given
		decoder := json.NewDecoder(strings.NewReader(`[]`))
		_, err := decoder.Token()
		require.NoError(t, err)

		// When
		err = validateValue(decoder, reflect.TypeFor[strictJSONFixture](), "$")

		// Then
		require.ErrorIs(t, err, ErrDelimiter)
	})

	t.Run("mismatched closing delimiter", func(t *testing.T) {
		// Given
		decoder := json.NewDecoder(strings.NewReader(`[]`))
		_, err := decoder.Token()
		require.NoError(t, err)

		// When
		err = consumeDelimiter(decoder, '}', "$.items")

		// Then
		require.ErrorIs(t, err, ErrDelimiter)
	})

	t.Run("missing closing delimiter", func(t *testing.T) {
		// Given
		decoder := json.NewDecoder(strings.NewReader(`[`))
		_, err := decoder.Token()
		require.NoError(t, err)

		// When
		err = consumeDelimiter(decoder, ']', "$.items")

		// Then
		require.Error(t, err)
		require.False(t, errors.Is(err, ErrDelimiter))
	})
}
