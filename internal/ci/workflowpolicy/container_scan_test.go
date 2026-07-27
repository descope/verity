package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInlineContainerImages_ignores_buildx_driver_and_port_values(t *testing.T) {
	// Given Docker CLI commands containing non-image values that resemble image tokens.
	run := `
docker buildx create --driver docker-container --driver-opt network=host
docker run --rm -p 127.0.0.1::8080:8080 alpine:latest true
`

	// When executable container images are extracted.
	images := inlineContainerImages(run)

	// Then only the docker run image is returned.
	assert.Equal(t, []string{"alpine:latest"}, images)
}
