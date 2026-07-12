package ci

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/melange"
)

type integerImpactOptions struct {
	ChangedFiles []string
	RepoRoot     string
	BaseLockPath string
	ImagesDir    string
}

type integerBuildLock struct {
	Packages      map[string]integerBuildLockPackage `json:"packages"`
	PipelineFiles map[string]string                  `json:"pipeline_files"`
}

type integerBuildLockPackage struct {
	File   string            `json:"file"`
	SHA256 string            `json:"sha256"`
	Assets map[string]string `json:"assets"`
}

type integerInputImpact struct {
	upstream  map[string]struct{}
	bespoke   map[string]struct{}
	overrides map[string]struct{}
	pipelines map[string]struct{}
}

func changedIntegerInputConsumers(opts integerImpactOptions) (map[string]struct{}, error) {
	impact := newIntegerInputImpact()
	paths := melange.DefaultPaths(opts.RepoRoot)
	head, err := collectIntegerInputImpact(&impact, &paths, opts)
	if err != nil {
		return nil, err
	}
	if len(impact.pipelines) > 0 {
		if err := addPipelineRecipeImpact(&impact, &paths, head); err != nil {
			return nil, err
		}
	}
	if len(impact.upstream)+len(impact.bespoke)+len(impact.overrides) == 0 {
		return map[string]struct{}{}, nil
	}
	return integerConsumers(opts.ImagesDir, impact)
}

func addLockDiffImpact(impact *integerInputImpact, base, head integerBuildLock) {
	for key := range unionKeys(base.Packages, head.Packages) {
		if !sameBuildLockPackage(base.Packages[key], head.Packages[key]) {
			impact.upstream[key] = struct{}{}
		}
	}
	for file := range unionKeys(base.PipelineFiles, head.PipelineFiles) {
		if base.PipelineFiles[file] != head.PipelineFiles[file] && filepath.Ext(file) == ".yaml" {
			impact.pipelines[strings.TrimSuffix(filepath.ToSlash(file), ".yaml")] = struct{}{}
		}
	}
}

func sameBuildLockPackage(left, right integerBuildLockPackage) bool {
	return left.File == right.File && left.SHA256 == right.SHA256 && maps.Equal(left.Assets, right.Assets)
}

func addLockedPathImpact(keys map[string]struct{}, changed string, lock integerBuildLock) bool {
	matched := false
	for key, entry := range lock.Packages {
		stem := strings.TrimSuffix(filepath.ToSlash(entry.File), filepath.Ext(entry.File))
		if changed == filepath.ToSlash(entry.File) || strings.HasPrefix(changed, stem+"/") {
			keys[key] = struct{}{}
			matched = true
			continue
		}
		if _, ok := entry.Assets[changed]; ok {
			keys[key] = struct{}{}
			matched = true
		}
	}
	return matched
}

func integerConsumers(imagesDir string, impact integerInputImpact) (map[string]struct{}, error) {
	files, err := intconfig.ImageFilePaths(imagesDir)
	if err != nil {
		return nil, err
	}
	consumers := map[string]struct{}{}
	for _, file := range files {
		def, err := intconfig.LoadImage(file)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", file, err)
		}
		versions := mapKeys(def.Versions)
		sort.Strings(versions)
		if len(versions) == 0 {
			versions = []string{"latest"}
		}
		matches, err := imageConsumesImpact(def, versions, impact)
		if err != nil {
			return nil, fmt.Errorf("resolve bespoke consumer %s: %w", def.Name, err)
		}
		if matches {
			consumers[def.Name] = struct{}{}
		}
	}
	return consumers, nil
}

func imageConsumesImpact(def *intconfig.ImageDef, versions []string, impact integerInputImpact) (bool, error) {
	for typeName := range def.Types {
		configSpec := def.Types[typeName].Melange
		if configSpec == nil {
			continue
		}
		for _, version := range versions {
			spec, err := melange.ResolveConfigSpec(configSpec, version)
			if err != nil {
				return false, fmt.Errorf("type %s version %s: %w", typeName, version, err)
			}
			if _, ok := impact.upstream[spec.Upstream]; ok {
				return true, nil
			}
			if _, ok := impact.overrides[spec.EnvFile]; ok {
				return true, nil
			}
			for _, file := range spec.Bespoke {
				if _, ok := impact.bespoke[file]; ok {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func loadIntegerBuildLock(path string) (integerBuildLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return integerBuildLock{}, fmt.Errorf("read bespoke lock: %w", err)
	}
	var lock integerBuildLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return integerBuildLock{}, fmt.Errorf("parse bespoke lock: %w", err)
	}
	if lock.Packages == nil {
		lock.Packages = map[string]integerBuildLockPackage{}
	}
	if lock.PipelineFiles == nil {
		lock.PipelineFiles = map[string]string{}
	}
	return lock, nil
}

func newIntegerInputImpact() integerInputImpact {
	return integerInputImpact{
		upstream:  map[string]struct{}{},
		bespoke:   map[string]struct{}{},
		overrides: map[string]struct{}{},
		pipelines: map[string]struct{}{},
	}
}

func unionKeys[V any](left, right map[string]V) map[string]struct{} {
	keys := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	return keys
}
