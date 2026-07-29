package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

func TestShardIntegerImages_preserves673EntriesInStableBoundedOrder(t *testing.T) {
	// Given
	images := syntheticIntegerImages(673)

	// When
	first, err := shardIntegerImages(images)
	require.NoError(t, err)
	second, err := shardIntegerImages(images)
	require.NoError(t, err)

	// Then
	require.Equal(t, first, second)
	require.Len(t, first, 11)
	assert.Equal(t, []int{64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 33}, shardCounts(first))

	flattened := flattenIntegerShards(t, first)
	require.Equal(t, images, flattened)
	seen := make(map[string]int, len(flattened))
	for _, image := range flattened {
		seen[image.Name+"\x00"+image.Version+"\x00"+image.Type]++
	}
	require.Len(t, seen, len(images))
	for identity, count := range seen {
		assert.Equal(t, 1, count, identity)
	}
}

func TestShardIntegerImages_preservesExistingSmallerMatrices(t *testing.T) {
	for _, count := range []int{0, 1, 63, 64} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			// Given
			images := syntheticIntegerImages(count)

			// When
			shards, err := shardIntegerImages(images)
			require.NoError(t, err)

			// Then
			if count == 0 {
				assert.Empty(t, shards)
				return
			}
			require.Len(t, shards, 1)
			assert.Equal(t, 1, shards[0].Shard)
			assert.Equal(t, count, shards[0].Count)
			assert.Equal(t, images, flattenIntegerShards(t, shards))
		})
	}
}

func TestShardIntegerImages_keepsWorkflowDispatchImageFilterScoped(t *testing.T) {
	// Given
	images := syntheticIntegerImages(673)
	filtered := filterIntegerImagesByName(images, "image-0042")

	// When
	shards, err := shardIntegerImages(filtered)
	require.NoError(t, err)

	// Then
	flattened := flattenIntegerShards(t, shards)
	require.Len(t, flattened, 1)
	assert.Equal(t, "image-0042", flattened[0].Name)
}

func TestAppendGitHubShardOutputTo_writesBoundedShardMatrix(t *testing.T) {
	// Given
	writer := &closeBuffer{}
	shards, err := shardIntegerImages(syntheticIntegerImages(65))
	require.NoError(t, err)
	data, err := json.Marshal(shards)
	require.NoError(t, err)

	// When
	err = appendGitHubShardOutputTo(writer, "out", len(shards), data)

	// Then
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(writer.Bytes(), []byte("shard_count=2\nshards<<__VERITY_INTEGER_SHARDS__\n")))
	assert.Contains(t, writer.String(), `"count":64`)
	assert.Contains(t, writer.String(), `"count":1`)
}

func shardCounts(shards []integerMatrixShard) []int {
	counts := make([]int, 0, len(shards))
	for _, shard := range shards {
		counts = append(counts, shard.Count)
	}
	return counts
}

func syntheticIntegerImages(count int) []intdiscovery.DiscoveredImage {
	images := make([]intdiscovery.DiscoveredImage, 0, count)
	for index := range count {
		images = append(images, intdiscovery.DiscoveredImage{
			Name:     fmt.Sprintf("image-%04d", index),
			Version:  strconv.Itoa(index),
			Type:     "default",
			Tags:     []string{strconv.Itoa(index)},
			Registry: "ghcr.io/verity-org",
		})
	}
	return images
}

func flattenIntegerShards(t *testing.T, shards []integerMatrixShard) []intdiscovery.DiscoveredImage {
	t.Helper()
	capacity := 0
	for _, shard := range shards {
		capacity += shard.Count
	}
	flattened := make([]intdiscovery.DiscoveredImage, 0, capacity)
	for _, shard := range shards {
		assert.LessOrEqual(t, shard.Count, ci.IntegerMatrixShardSize)
		var entries []intdiscovery.DiscoveredImage
		require.NoError(t, json.Unmarshal([]byte(shard.Entries), &entries))
		require.Len(t, entries, shard.Count)
		flattened = append(flattened, entries...)
	}
	return flattened
}
