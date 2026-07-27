package sitepublication

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDockerMountArgument_accepts_complete_readonly_and_writable_mounts(t *testing.T) {
	tests := []struct {
		name     string
		argument string
		readonly bool
	}{
		{name: "readonly", argument: "--mount=type=bind,src=/host/input,dst=/inputs/input,readonly", readonly: true},
		{name: "writable", argument: "--mount=type=bind,src=/host/output,dst=/output", readonly: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			mount, err := parseDockerMountArgument(test.argument)

			// Then
			require.NoError(t, err)
			assert.Equal(t, "/host/"+map[bool]string{true: "input", false: "output"}[test.readonly], mount.source)
			assert.Equal(t, map[bool]string{true: "/inputs/input", false: "/output"}[test.readonly], mount.destination)
			assert.Equal(t, test.readonly, mount.readonly)
		})
	}
}

func TestParseDockerMountArgument_rejects_ambiguous_or_unsupported_fields(t *testing.T) {
	tests := []struct {
		name     string
		argument string
	}{
		{name: "missing prefix", argument: "type=bind,src=/a,dst=/b"},
		{name: "missing destination", argument: "--mount=type=bind,src=/a"},
		{name: "invalid field", argument: "--mount=type=bind,src=/a,dst=/b,broken"},
		{name: "duplicate readonly", argument: "--mount=type=bind,src=/a,dst=/b,readonly,readonly"},
		{name: "duplicate type", argument: "--mount=type=bind,type=bind,src=/a,dst=/b"},
		{name: "wrong type", argument: "--mount=type=volume,src=/a,dst=/b"},
		{name: "duplicate source", argument: "--mount=type=bind,src=/a,src=/b,dst=/c"},
		{name: "duplicate destination", argument: "--mount=type=bind,src=/a,dst=/b,dst=/c"},
		{name: "empty value", argument: "--mount=type=bind,src=,dst=/b"},
		{name: "unsupported field", argument: "--mount=type=bind,src=/a,dst=/b,bind-propagation=rshared"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			mount, err := parseDockerMountArgument(test.argument)

			// Then
			require.ErrorIs(t, err, ErrInvalidSignerPlan)
			assert.Equal(t, dockerMountSpec{}, mount)
		})
	}
}

func TestSignerSafeEnvironment_keeps_first_allowlisted_value_and_drops_malformed_or_ambient_entries(t *testing.T) {
	// Given
	environment := []string{
		"MALFORMED",
		"HOME=/sentinel/home",
		"GH_TOKEN=first-token",
		"GH_TOKEN=second-token",
		"GITHUB_TOKEN=github-token",
		"SSL_CERT_FILE=/sentinel/cert.pem",
		"SSL_CERT_DIR=/sentinel/certs",
	}

	// When
	filtered := signerSafeEnvironment(environment)

	// Then
	assert.Equal(t, []string{
		"PATH=/usr/bin:/bin",
		"GH_TOKEN=first-token",
		"GITHUB_TOKEN=github-token",
		"SSL_CERT_FILE=/sentinel/cert.pem",
		"SSL_CERT_DIR=/sentinel/certs",
	}, filtered)
}
