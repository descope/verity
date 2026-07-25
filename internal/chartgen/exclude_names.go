package chartgen

import (
	"fmt"
	"os"
	"path/filepath"
)

func ImageDefinitionNames(dir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read image definitions %q: %w", dir, err)
	}

	names := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		names[entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]] = struct{}{}
	}
	return names, nil
}
