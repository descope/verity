package ci

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegerJSONParsers_reject_malformed_and_invalid_shapes(t *testing.T) {
	parsers := []struct {
		name  string
		parse func([]byte) error
	}{
		{name: "plan", parse: func(data []byte) error { _, err := ParseIntegerBatchPlan(data); return err }},
		{name: "component", parse: func(data []byte) error { _, err := ParseIntegerComponentManifest(data); return err }},
		{name: "inventory", parse: func(data []byte) error { _, err := ParseIntegerShardInventory(data); return err }},
		{name: "shard", parse: func(data []byte) error { _, err := ParseIntegerShardManifest(data); return err }},
		{name: "batch", parse: func(data []byte) error { _, err := ParseIntegerBatchManifest(data); return err }},
	}

	for _, parser := range parsers {
		for _, input := range []string{`[]`, `{}`} {
			t.Run(parser.name+"/"+input, func(t *testing.T) {
				// When
				err := parser.parse([]byte(input))

				// Then
				require.Error(t, err)
			})
		}
	}
}

func TestIntegerJSONDecoder_rejects_duplicate_trailing_and_truncated_values(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "nested duplicate", content: `{"outer":{"key":1,"key":2}}`},
		{name: "trailing value", content: `{"outer":1} {}`},
		{name: "malformed trailing value", content: `{"outer":1} x`},
		{name: "truncated object", content: `{"outer":`},
		{name: "truncated array", content: `{"outer":[`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			var destination map[string]json.RawMessage

			// When
			err := decodeIntegerJSON([]byte(test.content), &destination)

			// Then
			require.Error(t, err)
		})
	}
}

func TestIntegerJSONDecoder_accepts_nested_arrays_and_closing_delimiters(t *testing.T) {
	// Given
	var destination map[string][]map[string]int

	// When
	err := decodeIntegerJSON([]byte(`{"outer":[{"value":1}]}`), &destination)

	// Then
	require.NoError(t, err)
	require.Equal(t, 1, destination["outer"][0]["value"])

	// Given
	decoder := json.NewDecoder(strings.NewReader(`[]`))
	_, err = decoder.Token()
	require.NoError(t, err)

	// When
	err = walkIntegerJSON(decoder, false)

	// Then
	require.NoError(t, err)
}

func TestIntegerJSONDecoder_rejects_unexpected_delimiter_and_unsupported_value(t *testing.T) {
	// Given
	decoder := json.NewDecoder(strings.NewReader(`[]`))
	_, err := decoder.Token()
	require.NoError(t, err)

	// When
	err = walkIntegerJSON(decoder, true)

	// Then
	require.ErrorIs(t, err, errIntegerJSONDelimiter)

	// When
	_, err = marshalIntegerJSON(make(chan int))

	// Then
	require.ErrorContains(t, err, "marshal Integer JSON")
}
