package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/verity-org/verity/internal/ci"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

type integerMatrixShard struct {
	Shard   int    `json:"shard"`
	Count   int    `json:"count"`
	Entries string `json:"entries"`
}

func shardIntegerImages(images []intdiscovery.DiscoveredImage) ([]integerMatrixShard, error) {
	shards := make([]integerMatrixShard, 0, (len(images)+ci.IntegerMatrixShardSize-1)/ci.IntegerMatrixShardSize)
	for start := 0; start < len(images); start += ci.IntegerMatrixShardSize {
		end := min(start+ci.IntegerMatrixShardSize, len(images))
		entries, err := json.Marshal(images[start:end])
		if err != nil {
			return nil, fmt.Errorf("marshalling Integer matrix shard %d: %w", len(shards)+1, err)
		}
		shards = append(shards, integerMatrixShard{
			Shard:   len(shards) + 1,
			Count:   end - start,
			Entries: string(entries),
		})
	}
	return shards, nil
}

func appendGitHubShardOutput(path string, shards []integerMatrixShard) error {
	data, err := json.Marshal(shards)
	if err != nil {
		return fmt.Errorf("marshalling Integer shard matrix: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening GitHub output %s: %w", path, err)
	}
	return appendGitHubShardOutputTo(f, path, len(shards), data)
}

func appendGitHubShardOutputTo(w io.WriteCloser, path string, count int, data []byte) (retErr error) {
	defer func() {
		if closeErr := w.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing GitHub output %s: %w", path, closeErr))
		}
	}()
	if _, err := fmt.Fprintf(w, "shard_count=%d\nshards<<__VERITY_INTEGER_SHARDS__\n%s\n__VERITY_INTEGER_SHARDS__\n", count, data); err != nil {
		return fmt.Errorf("writing GitHub output %s: %w", path, err)
	}
	return nil
}
