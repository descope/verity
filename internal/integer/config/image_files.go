package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
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

// LoadedImage is one parsed image definition and its repository path.
type LoadedImage struct {
	Path       string
	Definition *ImageDef
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
	root, err := os.OpenRoot(imagesDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	entries, err := imageFileEntries(root, imagesDir)
	if err != nil {
		return nil, err
	}
	loaded := make([]LoadedImage, 0, len(entries))
	pathsByName := make(map[string]string, len(entries))
	for _, entry := range entries {
		definition, err := loadImageEntry(root, entry)
		if err != nil {
			return nil, err
		}
		if previous, exists := pathsByName[definition.Name]; definition.Name != "" && exists {
			return nil, fmt.Errorf("%w %q: %q and %q", ErrDuplicateImageName, definition.Name, previous, entry.path)
		}
		pathsByName[definition.Name] = entry.path
		loaded = append(loaded, LoadedImage{Path: entry.path, Definition: definition})
	}
	return loaded, nil
}

func imageFileEntries(root *os.Root, imagesDir string) ([]imageFileEntry, error) {
	entries := []imageFileEntry{}
	err := fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
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
				return fmt.Errorf("stat image definition %q: %w", filepath.Join(imagesDir, filepath.FromSlash(path)), err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%w %q", ErrInvalidImageFile, filepath.Join(imagesDir, filepath.FromSlash(path)))
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
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})
	return entries, nil
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
