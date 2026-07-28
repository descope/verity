package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntegerMatrixShard_startsANewShardAfter64Targets(t *testing.T) {
	// Given
	indices := []int{0, 63, 64, 127, 128}

	// When
	shards := make([]int, 0, len(indices))
	for _, index := range indices {
		shards = append(shards, IntegerMatrixShard(index))
	}

	// Then
	assert.Equal(t, []int{1, 1, 2, 2, 3}, shards)
}
