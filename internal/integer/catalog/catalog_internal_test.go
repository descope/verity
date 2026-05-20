package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildVariantFallbackPreservesTypeSuffix(t *testing.T) {
	variant := buildVariant("node", "24", "dev", "ghcr.io/verity-org", "", nil)

	assert.Equal(t, []string{"24-dev"}, variant.Tags)
	assert.Equal(t, "ghcr.io/verity-org/node:24-dev", variant.Ref)
}
