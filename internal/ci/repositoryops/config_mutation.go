package repositoryops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
)

var ErrInvalidCatalog = errors.New("invalid copa catalog")

func AppendStandaloneImage(path string, issue ImageIssue) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, fmt.Errorf("inspect catalog %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%w: %q is not a regular file", ErrInvalidCatalog, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read catalog %q: %w", path, err)
	}
	var catalog config.CopaConfig
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return false, fmt.Errorf("%w: %w", ErrInvalidCatalog, err)
	}
	for index := range catalog.Images {
		image := &catalog.Images[index]
		if image.Name == issue.name {
			return true, nil
		}
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return false, fmt.Errorf("%w: %w", ErrInvalidCatalog, err)
	}
	images, err := imageSequence(&document)
	if err != nil {
		return false, err
	}
	imageNode, err := standaloneImageNode(issue)
	if err != nil {
		return false, err
	}
	images.Content = append(images.Content, imageNode)
	if err := writeYAMLAtomically(path, info.Mode().Perm(), &document); err != nil {
		return false, err
	}
	return false, nil
}

func imageSequence(document *yaml.Node) (*yaml.Node, error) {
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: root must be a mapping", ErrInvalidCatalog)
	}
	root := document.Content[0]
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value != "images" {
			continue
		}
		images := root.Content[index+1]
		if images.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("%w: images must be a sequence", ErrInvalidCatalog)
		}
		return images, nil
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "images"}
	images := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	root.Content = append(root.Content, key, images)
	return images, nil
}

func standaloneImageNode(issue ImageIssue) (*yaml.Node, error) {
	entry := config.ImageSpec{
		Name:      issue.name,
		Image:     issue.ImageRepository(),
		Platforms: []string{"linux/amd64", "linux/arm64"},
		Tags: config.TagStrategy{
			Strategy: "pattern",
			Pattern:  `^\d+\.\d+\.\d+$`,
			MaxTags:  3,
		},
	}
	data, err := yaml.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("marshal catalog image: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse catalog image node: %w", err)
	}
	if len(document.Content) != 1 {
		return nil, fmt.Errorf("%w: generated image node is empty", ErrInvalidCatalog)
	}
	return document.Content[0], nil
}

func writeYAMLAtomically(path string, mode os.FileMode, document *yaml.Node) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".copa-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary catalog: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := temporary.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("close temporary catalog: %w", closeErr)
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
			err = fmt.Errorf("clean temporary catalog: %w", removeErr)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary catalog mode: %w", err)
	}
	encoder := yaml.NewEncoder(temporary)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode catalog: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("close catalog encoder: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync catalog: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close catalog: %w", err)
	}
	closed = true
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace catalog: %w", err)
	}
	return nil
}
