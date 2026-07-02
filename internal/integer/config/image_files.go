package config

import (
	"io/fs"
	"path/filepath"
	"sort"
)

// ImageFilePaths returns every image definition below imagesDir, excluding
// shared base fragments under _base/.
func ImageFilePaths(imagesDir string) ([]string, error) {
	paths := []string{}
	err := filepath.WalkDir(imagesDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "_base" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".yaml" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
