package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

var (
	// ErrImageNotFound is returned when no definition declares the requested name.
	ErrImageNotFound = errors.New("image definition not found")

	// ErrDuplicateImageName is returned when multiple definitions declare the same name.
	ErrDuplicateImageName = errors.New("duplicate image definition name")

	// ErrInvalidImageFile is returned when an image definition is not a regular file.
	ErrInvalidImageFile = errors.New("image definition must be a regular file")
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
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("stat image definition %q: %w", path, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%w %q", ErrInvalidImageFile, path)
			}
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

// LoadImageByName finds the single regular image definition with the requested declared name.
func LoadImageByName(imagesDir, name string) (*ImageDef, error) {
	paths, err := ImageFilePaths(imagesDir)
	if err != nil {
		return nil, fmt.Errorf("list image definitions: %w", err)
	}
	var match *ImageDef
	matchPath := ""
	for _, path := range paths {
		candidate, err := LoadImage(path)
		if err != nil {
			return nil, err
		}
		if candidate.Name != name {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("%w %q: %q and %q", ErrDuplicateImageName, name, matchPath, path)
		}
		match = candidate
		matchPath = path
	}
	if match == nil {
		return nil, fmt.Errorf("%w %q under %q", ErrImageNotFound, name, imagesDir)
	}
	return match, nil
}
