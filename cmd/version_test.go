package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/buildmetadata"
)

func TestWriteHumanVersion_is_concise(t *testing.T) {
	// Given complete build metadata.
	info := buildmetadata.Info{Version: "v9-custom"}
	var output bytes.Buffer

	// When the human renderer writes the version.
	err := writeHumanVersion(&output, &info)

	// Then only the concise product name and version are emitted.
	require.NoError(t, err)
	assert.Equal(t, "verity v9-custom\n", output.String())
}

func TestWriteVersionJSON_emits_machine_readable_metadata(t *testing.T) {
	// Given complete metadata for a release build.
	info := buildmetadata.Info{
		Version:     "v9-custom",
		SourceSHA:   "0123456789012345678901234567890123456789",
		BuildKey:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		GoVersion:   "go1.26.5",
		GOOS:        "linux",
		GOARCH:      "amd64",
		CGOEnabled:  "0",
		BuildFlags:  []string{"-trimpath"},
		BuildStatus: buildmetadata.BuiltStatus,
	}
	var output bytes.Buffer

	// When the JSON renderer writes the metadata.
	err := writeVersionJSON(&output, &info)

	// Then the output is one valid JSON object with the trusted identity fields.
	require.NoError(t, err)
	var decoded buildmetadata.Info
	require.NoError(t, json.Unmarshal(output.Bytes(), &decoded))
	assert.Equal(t, info, decoded)
	assert.Equal(t, byte('\n'), output.Bytes()[len(output.Bytes())-1])
}
