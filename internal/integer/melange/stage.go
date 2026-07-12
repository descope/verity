package melange

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type lockFile struct {
	Packages      map[string]lockPackage `json:"packages"`
	PipelineFiles map[string]string      `json:"pipeline_files"`
}

type lockPackage struct {
	File   string            `json:"file"`
	SHA256 string            `json:"sha256"`
	Assets map[string]string `json:"assets"`
}

type stageDestination struct {
	root    *os.Root
	workDir string
}

type stageContext struct {
	paths       *Paths
	lock        lockFile
	destination stageDestination
}

var stageAfterManagedRoot func()

func Stage(paths *Paths, spec Spec) error {
	if !spec.Needed() {
		return errEmptySpec
	}
	lock, err := loadLock(paths.LockFile)
	if err != nil {
		return err
	}
	workRoot, workDir, err := ensureManagedDirectory(paths.Root, paths.WorkDir)
	if err != nil {
		return fmt.Errorf("prepare work directory: %w", err)
	}
	defer workRoot.Close()
	stage := stageContext{
		paths: paths,
		lock:  lock,
		destination: stageDestination{
			root:    workRoot,
			workDir: workDir,
		},
	}
	if stageAfterManagedRoot != nil {
		stageAfterManagedRoot()
	}
	if err := stage.destination.reset("specs"); err != nil {
		return fmt.Errorf("reset staged specs: %w", err)
	}
	if spec.Upstream != "" {
		if err := stage.lockedRecipe(spec.Upstream); err != nil {
			return err
		}
	} else if err := stage.bespokeRecipes(spec.Bespoke); err != nil {
		return err
	}
	return stage.pipelines(lock.PipelineFiles)
}

func loadLock(path string) (lockFile, error) {
	data, err := readRegularFile(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		return lockFile{}, fmt.Errorf("read lock file: %w", err)
	}
	var lock lockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return lockFile{}, fmt.Errorf("parse lock file: %w", err)
	}
	return lock, nil
}

func (s *stageContext) lockedRecipe(key string) error {
	entry, ok := s.lock.Packages[key]
	if !ok || entry.File == "" || entry.SHA256 == "" {
		return fmt.Errorf("%w: %q", errMissingLockMetadata, key)
	}
	recipe, err := readVerifiedFile(s.paths.LockedDir, entry.File, entry.SHA256)
	if err != nil {
		return fmt.Errorf("verify recipe %s: %w", key, err)
	}
	sidecarName := strings.TrimSuffix(entry.File, filepath.Ext(entry.File))
	sidecarDir, err := secureOptionalDir(s.paths.LockedDir, sidecarName)
	if err != nil {
		return fmt.Errorf("verify recipe %s sidecar: %w", key, err)
	}
	actual, err := treeFiles(sidecarDir, filepath.ToSlash(sidecarName))
	if err != nil {
		return fmt.Errorf("list recipe %s sidecar: %w", key, err)
	}
	if err := compareFileSet("recipe "+key+" sidecar", mapKeys(entry.Assets), actual); err != nil {
		return err
	}
	assets := make(map[string][]byte, len(entry.Assets))
	for asset, expected := range entry.Assets {
		data, err := readVerifiedFile(s.paths.LockedDir, asset, expected)
		if err != nil {
			return fmt.Errorf("verify recipe asset %s: %w", asset, err)
		}
		assets[asset] = data
	}
	dest := filepath.Join("specs", key)
	if err := s.destination.write(filepath.Join(dest, "build.yaml"), recipe); err != nil {
		return err
	}
	prefix := filepath.ToSlash(sidecarName) + "/"
	for asset, data := range assets {
		normalized := filepath.ToSlash(asset)
		relative := strings.TrimPrefix(normalized, prefix)
		if relative == normalized {
			return fmt.Errorf("recipe asset %s %w", asset, errUnsafeRelativePath)
		}
		if err := s.destination.write(filepath.Join(dest, filepath.FromSlash(relative)), data); err != nil {
			return err
		}
	}
	return nil
}

func (s *stageContext) bespokeRecipes(files []string) error {
	for _, file := range files {
		data, err := readRegularFile(s.paths.BespokeDir, file)
		if err != nil {
			return fmt.Errorf("verify bespoke recipe %s: %w", file, err)
		}
		dest := filepath.Join("specs", file, "build.yaml")
		if err := s.destination.write(dest, data); err != nil {
			return err
		}
	}
	return nil
}

func (s *stageContext) pipelines(expected map[string]string) error {
	actual, err := treeFiles(s.paths.PipelinesDir, "")
	if err != nil {
		return fmt.Errorf("list shared pipelines: %w", err)
	}
	if err := compareFileSet("pipeline", mapKeys(expected), actual); err != nil {
		return err
	}
	files := make(map[string][]byte, len(expected))
	for file, digest := range expected {
		data, err := readVerifiedFile(s.paths.PipelinesDir, file, digest)
		if err != nil {
			return fmt.Errorf("verify pipeline %s: %w", file, err)
		}
		files[file] = data
	}
	if err := s.destination.reset("pipelines"); err != nil {
		return fmt.Errorf("reset staged pipelines: %w", err)
	}
	if len(actual) == 0 {
		return nil
	}
	for file, data := range files {
		if err := s.destination.write(filepath.Join("pipelines", filepath.FromSlash(file)), data); err != nil {
			return err
		}
	}
	return nil
}

func (d *stageDestination) reset(relative string) error {
	path := filepath.Join(d.workDir, relative)
	if err := d.root.RemoveAll(path); err != nil {
		return err
	}
	return d.root.MkdirAll(path, 0o755)
}

func (d *stageDestination) write(relative string, data []byte) error {
	path := filepath.Join(d.workDir, relative)
	if err := d.root.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := d.root.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
