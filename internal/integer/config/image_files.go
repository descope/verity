package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	// ErrImageNotFound is returned when no definition declares the requested name.
	ErrImageNotFound = errors.New("image definition not found")

	// ErrDuplicateImageName is returned when multiple definitions declare the same name.
	ErrDuplicateImageName = errors.New("duplicate image definition name")

	// ErrInvalidImageFile is returned when an image definition is not a regular file.
	ErrInvalidImageFile = errors.New("image definition must be a regular file")
)

// LoadedImage is one parsed image definition and its repository path.
type LoadedImage struct {
	Path       string
	Definition *ImageDef
}

// ImageDefinitionLoadError records one definition that could not be loaded.
type ImageDefinitionLoadError struct {
	Path string
	Err  error
}

func (e ImageDefinitionLoadError) Error() string {
	return e.Err.Error()
}

func (e ImageDefinitionLoadError) Unwrap() error {
	return e.Err
}

type imageFileEntry struct {
	path     string
	relative string
	info     fs.FileInfo
}

// ImageFilePaths returns every image definition below imagesDir, excluding
// shared base fragments under _base/.
func ImageFilePaths(imagesDir string) ([]string, error) {
	root, err := os.OpenRoot(imagesDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	entries, err := imageFileEntries(root, imagesDir)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.path)
	}
	return paths, nil
}

// LoadImageDefinitions returns every parsed image definition with unique declared names.
func LoadImageDefinitions(imagesDir string) ([]LoadedImage, error) {
	loaded, failures, err := LoadImageDefinitionsBestEffort(imagesDir)
	if err != nil {
		return nil, err
	}
	if err := joinImageDefinitionLoadErrors(failures); err != nil {
		return nil, err
	}
	return loaded, nil
}

// LoadImageDefinitionsBestEffort returns every unambiguous parsed definition
// and reports per-definition failures without discarding unrelated images.
func LoadImageDefinitionsBestEffort(imagesDir string) ([]LoadedImage, []ImageDefinitionLoadError, error) {
	root, err := os.OpenRoot(imagesDir)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()

	entries, failures, err := imageFileEntriesBestEffort(root, imagesDir)
	if err != nil {
		return nil, nil, err
	}
	candidates := make([]LoadedImage, 0, len(entries))
	pathsByName := make(map[string][]string, len(entries))
	for _, entry := range entries {
		definition, err := loadImageEntry(root, entry)
		if err != nil {
			failures = append(failures, ImageDefinitionLoadError{Path: entry.path, Err: err})
			continue
		}
		candidates = append(candidates, LoadedImage{Path: entry.path, Definition: definition})
		if definition.Name != "" {
			pathsByName[definition.Name] = append(pathsByName[definition.Name], entry.path)
		}
	}

	loaded := make([]LoadedImage, 0, len(candidates))
	reportedDuplicates := make(map[string]struct{})
	for _, candidate := range candidates {
		paths := pathsByName[candidate.Definition.Name]
		if candidate.Definition.Name == "" || len(paths) == 1 {
			loaded = append(loaded, candidate)
			continue
		}
		if _, reported := reportedDuplicates[candidate.Definition.Name]; reported {
			continue
		}
		reportedDuplicates[candidate.Definition.Name] = struct{}{}
		quotedPaths := make([]string, len(paths))
		for index, path := range paths {
			quotedPaths[index] = fmt.Sprintf("%q", path)
		}
		failures = append(failures, ImageDefinitionLoadError{
			Path: paths[0],
			Err:  fmt.Errorf("%w %q: %s", ErrDuplicateImageName, candidate.Definition.Name, strings.Join(quotedPaths, " and ")),
		})
	}
	sort.SliceStable(failures, func(i, j int) bool {
		return failures[i].Path < failures[j].Path
	})
	return loaded, failures, nil
}

func imageFileEntries(root *os.Root, imagesDir string) ([]imageFileEntry, error) {
	entries, failures, err := imageFileEntriesBestEffort(root, imagesDir)
	if err != nil {
		return nil, err
	}
	if err := joinImageDefinitionLoadErrors(failures); err != nil {
		return nil, err
	}
	return entries, nil
}

func imageFileEntriesBestEffort(root *os.Root, imagesDir string) ([]imageFileEntry, []ImageDefinitionLoadError, error) {
	entries := []imageFileEntry{}
	failures := []ImageDefinitionLoadError{}
	err := fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == "." {
				return err
			}
			failures = append(failures, ImageDefinitionLoadError{
				Path: filepath.Join(imagesDir, filepath.FromSlash(path)),
				Err:  err,
			})
			return nil
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
				fullPath := filepath.Join(imagesDir, filepath.FromSlash(path))
				failures = append(failures, ImageDefinitionLoadError{
					Path: fullPath,
					Err:  fmt.Errorf("stat image definition %q: %w", fullPath, err),
				})
				return nil
			}
			if !info.Mode().IsRegular() {
				fullPath := filepath.Join(imagesDir, filepath.FromSlash(path))
				failures = append(failures, ImageDefinitionLoadError{
					Path: fullPath,
					Err:  fmt.Errorf("%w %q", ErrInvalidImageFile, fullPath),
				})
				return nil
			}
			entries = append(entries, imageFileEntry{
				path:     filepath.Join(imagesDir, filepath.FromSlash(path)),
				relative: filepath.Clean(filepath.FromSlash(path)),
				info:     info,
			})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})
	sort.SliceStable(failures, func(i, j int) bool {
		return failures[i].Path < failures[j].Path
	})
	return entries, failures, nil
}

func joinImageDefinitionLoadErrors(failures []ImageDefinitionLoadError) error {
	errs := make([]error, 0, len(failures))
	for _, failure := range failures {
		errs = append(errs, failure)
	}
	return errors.Join(errs...)
}

func loadImageEntry(root *os.Root, entry imageFileEntry) (*ImageDef, error) {
	file, err := root.Open(entry.relative)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrInvalidImageFile, entry.path, err)
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat opened image definition %q: %w", entry.path, err)
	}
	currentInfo, err := root.Lstat(entry.relative)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrInvalidImageFile, entry.path, err)
	}
	if !openedInfo.Mode().IsRegular() || !currentInfo.Mode().IsRegular() ||
		!os.SameFile(entry.info, openedInfo) || !os.SameFile(openedInfo, currentInfo) {
		return nil, fmt.Errorf("%w %q", ErrInvalidImageFile, entry.path)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("reading image %q: %w", entry.path, err)
	}
	return parseImage(data, entry.path)
}

// LoadImageByName finds the single regular image definition with the requested declared name.
func LoadImageByName(imagesDir, name string) (*ImageDef, error) {
	images, err := LoadImageDefinitions(imagesDir)
	if err != nil {
		return nil, fmt.Errorf("list image definitions: %w", err)
	}
	for _, image := range images {
		if image.Definition.Name == name {
			return image.Definition, nil
		}
	}
	return nil, fmt.Errorf("%w %q under %q", ErrImageNotFound, name, imagesDir)
}
