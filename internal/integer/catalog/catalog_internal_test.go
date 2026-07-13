package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildVariantFallbackPreservesTypeSuffix(t *testing.T) {
	variant := buildVariant("node", "24", "dev", "verity.supply", "", nil)

	assert.Equal(t, []string{"24-dev"}, variant.Tags)
	assert.Equal(t, "verity.supply/node:24-dev", variant.Ref)
}
