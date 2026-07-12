package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

var (
	// ErrImageNotFound is returned when no definition declares the requested name.
	ErrImageNotFound = errors.New("image definition not found")

	// ErrDuplicateImageName is returned when multiple definitions declare the same name.
	ErrDuplicateImageName = errors.New("duplicate image definition name")
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

// LoadImageByName finds an image definition by its declared name. The direct
// images/<name>.yaml path remains the fast path; nested or renamed definitions
// fall back to the recursive image inventory.
func LoadImageByName(imagesDir, name string) (*ImageDef, error) {
	relative := filepath.Clean(filepath.FromSlash(name) + ".yaml")
	directOK := !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	if directOK {
		direct, err := LoadImage(filepath.Join(imagesDir, relative))
		if err == nil && direct.Name == name {
			return direct, nil
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return findImageByDeclaredName(imagesDir, name)
}

func findImageByDeclaredName(imagesDir, name string) (*ImageDef, error) {
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
