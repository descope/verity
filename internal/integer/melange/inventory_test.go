package melange

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func TestLockedRecipeInventoryComplete(t *testing.T) {
	paths := repositoryTestPaths(t)
	lock, err := loadLock(paths.LockFile)
	require.NoError(t, err)

	for name, entry := range lock.Packages {
		t.Run(name, func(t *testing.T) {
			_, verifyErr := readVerifiedFile(paths.LockedDir, entry.File, entry.SHA256)
			require.NoError(t, verifyErr)
			for asset, digest := range entry.Assets {
				_, verifyErr = readVerifiedFile(paths.LockedDir, asset, digest)
				require.NoError(t, verifyErr)
			}
		})
	}
}

func TestEveryMelangeUpstreamConsumerHasLockedRecipe(t *testing.T) {
	paths := repositoryTestPaths(t)
	lock, err := loadLock(paths.LockFile)
	require.NoError(t, err)

	err = filepath.WalkDir(paths.ImagesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".yaml" || strings.HasPrefix(entry.Name(), "_") {
			return nil
		}
		def, loadErr := intconfig.LoadImage(path)
		if loadErr != nil {
			return loadErr
		}
		for typeName, imageType := range def.Types {
			if len(def.Versions) == 0 {
				if imageType.Melange != nil && imageType.Melange.Upstream != "" {
					requireLockedConsumer(t, lock, def.Name, typeName, "latest", imageType.Melange)
				}
				continue
			}
			for version, metadata := range def.Versions {
				if slices.Contains(metadata.SkipTypes, typeName) {
					continue
				}
				configSpec := def.MelangeFor(version, typeName)
				if configSpec != nil && configSpec.Upstream != "" {
					requireLockedConsumer(t, lock, def.Name, typeName, version, configSpec)
				}
			}
		}
		return nil
	})
	require.NoError(t, err)
}

func TestCustomPipelineInventoryComplete(t *testing.T) {
	paths := repositoryTestPaths(t)
	lock, err := loadLock(paths.LockFile)
	require.NoError(t, err)

	for file, digest := range lock.PipelineFiles {
		_, verifyErr := readVerifiedFile(paths.PipelinesDir, file, digest)
		require.NoError(t, verifyErr, file)
	}

	for _, root := range []string{paths.BespokeDir, paths.PipelinesDir} {
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".yaml" {
				return nil
			}
			uses, usesErr := pipelineUses(path)
			if usesErr != nil {
				return usesErr
			}
			for name := range uses {
				if !isCustomPipeline(name) {
					continue
				}
				require.Contains(t, lock.PipelineFiles, name+".yaml", "%s references %s", path, name)
			}
			return nil
		})
		require.NoError(t, err)
	}
}

func TestPackagingInputsDoNotCheckoutMutableBranches(t *testing.T) {
	// Given: every repository-owned recipe and custom pipeline.
	paths := repositoryTestPaths(t)

	// When: its YAML is inspected for git checkouts of mutable default branches.
	for _, root := range []string{paths.BespokeDir, paths.PipelinesDir} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".yaml" {
				return nil
			}
			relative, relativeErr := filepath.Rel(root, path)
			if relativeErr != nil {
				return relativeErr
			}
			data, readErr := readRegularFile(root, relative)
			if readErr != nil {
				return readErr
			}
			var document yaml.Node
			if unmarshalErr := yaml.Unmarshal(data, &document); unmarshalErr != nil {
				return unmarshalErr
			}

			// Then: every external checkout is content-addressed or version-addressed.
			require.False(t, containsMutableGitCheckout(&document), path)
			return nil
		})
		require.NoError(t, err)
	}
}

func pipelineUses(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	uses := map[string]struct{}{}
	collectPipelineUses(&root, uses)
	return uses, nil
}

func collectPipelineUses(node *yaml.Node, uses map[string]struct{}) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Value == "uses" && value.Kind == yaml.ScalarNode {
				uses[value.Value] = struct{}{}
			}
			collectPipelineUses(value, uses)
		}
		return
	}
	for _, child := range node.Content {
		collectPipelineUses(child, uses)
	}
}

func containsMutableGitCheckout(node *yaml.Node) bool {
	if node.Kind == yaml.MappingNode {
		uses := yamlMappingValue(node, "uses")
		with := yamlMappingValue(node, "with")
		var branch *yaml.Node
		if with != nil {
			branch = yamlMappingValue(with, "branch")
		}
		if uses != nil && uses.Value == "git-checkout" && branch != nil && (branch.Value == "main" || branch.Value == "master") {
			return true
		}
	}
	return slices.ContainsFunc(node.Content, containsMutableGitCheckout)
}

func isCustomPipeline(name string) bool {
	return name == "go/bump" || strings.HasPrefix(name, "auth/") || strings.HasPrefix(name, "iamguarded/") || strings.HasPrefix(name, "test/")
}

func requireLockedConsumer(t *testing.T, lock lockFile, image, typeName, version string, configSpec *intconfig.MelangeSpec) {
	t.Helper()
	spec, err := ResolveConfigSpec(configSpec, version)
	require.NoError(t, err, "%s:%s-%s", image, version, typeName)
	require.Contains(t, lock.Packages, spec.Upstream, "%s:%s-%s", image, version, typeName)
}

func repositoryTestPaths(t *testing.T) Paths {
	t.Helper()
	workingDir, err := os.Getwd()
	require.NoError(t, err)
	return DefaultPaths(filepath.Clean(filepath.Join(workingDir, "..", "..", "..")))
}
