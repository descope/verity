package discovery_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verity-org/verity/internal/integer/discovery"
)

func TestFindLatestVersion_prefersLatestWhenOnlyVariantTagsExist(t *testing.T) {
	versions := []string{"debug", "debug-nonroot", "latest", "nonroot"}

	assert.Equal(t, "latest", discovery.FindLatestVersion(versions))
}

func TestFindLatestVersion_prefersHighestNumericStream(t *testing.T) {
	versions := []string{"latest", "22", "24"}

	assert.Equal(t, "24", discovery.FindLatestVersion(versions))
}

func TestFindLatestVersion_noNumericNoLatestReturnsEmpty(t *testing.T) {
	versions := []string{"debug", "nonroot"}

	assert.Equal(t, "", discovery.FindLatestVersion(versions))
}
