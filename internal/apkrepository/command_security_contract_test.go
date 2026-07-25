package apkrepository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandEnvironment_rejects_signing_key_and_runtime_inheritance(t *testing.T) {
	// Given a hostile parent environment containing the legacy key and executable overrides.
	parent := []string{
		"APK_REPOSITORY_PRIVATE_KEY=sentinel-must-not-inherit",
		"PATH=/attacker/bin",
		"LD_PRELOAD=/attacker/library.so",
		"DOCKER_HOST=tcp://attacker.invalid:2375",
		"GH_TOKEN=github-token",
		"SSL_CERT_FILE=/etc/ssl/cert.pem",
	}

	// When environments are constructed for the signer tool and GitHub client.
	melange := commandEnvironment("melange", parent)
	github := commandEnvironment("gh", parent)

	// Then children receive only fixed PATH and explicitly allowlisted non-key values.
	assert.Equal(t, []string{"PATH=/usr/bin:/bin:/sbin", "SSL_CERT_FILE=/etc/ssl/cert.pem"}, melange)
	assert.Equal(t, []string{"PATH=/usr/bin:/bin:/sbin", "GH_TOKEN=github-token", "SSL_CERT_FILE=/etc/ssl/cert.pem"}, github)
	for _, environment := range [][]string{melange, github} {
		assert.NotContains(t, environment, "APK_REPOSITORY_PRIVATE_KEY=sentinel-must-not-inherit")
		assert.NotContains(t, environment, "LD_PRELOAD=/attacker/library.so")
		assert.NotContains(t, environment, "DOCKER_HOST=tcp://attacker.invalid:2375")
	}
}

func TestTrustedCommandPath_rejects_PATH_resolved_signing_tools(t *testing.T) {
	// Given signer-related command names.

	// When their executable paths are resolved.
	melange := trustedCommandPath("melange")
	apk := trustedCommandPath("apk")

	// Then fixed absolute binaries are used instead of parent PATH aliases.
	assert.Equal(t, "/usr/bin/melange", melange)
	assert.Equal(t, "/sbin/apk", apk)
}
