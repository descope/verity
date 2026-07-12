package ci

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/verity-org/verity/internal/integer/melange"
)

var (
	errBaseIntegerLockRequired = errors.New("base bespoke lock is required when packages/upstream.lock.json changes")
	errUnknownLockedInput      = errors.New("changed locked recipe input is not present in the bespoke lock")
)

type integerImpactLocks struct {
	head integerBuildLock
	base integerBuildLock
}

func collectIntegerInputImpact(impact *integerInputImpact, paths *melange.Paths, opts integerImpactOptions) (integerBuildLock, error) {
	lockChanged := containsNormalizedPath(opts.ChangedFiles, "packages/upstream.lock.json")
	locks, err := loadIntegerImpactLocks(paths, opts, lockChanged)
	if err != nil {
		return locks.head, err
	}
	if lockChanged {
		addLockDiffImpact(impact, locks.base, locks.head)
	}
	if err := addChangedInputPaths(impact, opts.ChangedFiles, locks.head, locks.base); err != nil {
		return locks.head, err
	}
	return locks.head, nil
}

func loadIntegerImpactLocks(paths *melange.Paths, opts integerImpactOptions, lockChanged bool) (integerImpactLocks, error) {
	var locks integerImpactLocks
	needsHead := lockChanged || pathsUnder(opts.ChangedFiles, "packages/bespoke/locked/") || pathsUnder(opts.ChangedFiles, "packages/pipelines/")
	var err error
	if needsHead {
		locks.head, err = loadIntegerBuildLock(paths.LockFile)
		if err != nil {
			return locks, err
		}
	}
	if !lockChanged {
		return locks, nil
	}
	if strings.TrimSpace(opts.BaseLockPath) == "" {
		return locks, errBaseIntegerLockRequired
	}
	locks.base, err = loadIntegerBuildLock(opts.BaseLockPath)
	if err != nil {
		return locks, fmt.Errorf("load base bespoke lock: %w", err)
	}
	return locks, nil
}

func addChangedInputPaths(impact *integerInputImpact, files []string, head, base integerBuildLock) error {
	for _, file := range files {
		if err := addChangedInputPath(impact, filepath.ToSlash(strings.TrimSpace(file)), head, base); err != nil {
			return err
		}
	}
	return nil
}

func addChangedInputPath(impact *integerInputImpact, file string, head, base integerBuildLock) error {
	switch {
	case strings.HasPrefix(file, "packages/bespoke/locked/"):
		rel := strings.TrimPrefix(file, "packages/bespoke/locked/")
		matched := addLockedPathImpact(impact.upstream, rel, head)
		matched = addLockedPathImpact(impact.upstream, rel, base) || matched
		if !matched {
			return fmt.Errorf("%w: %s", errUnknownLockedInput, file)
		}
	case strings.HasPrefix(file, "packages/bespoke/"):
		rel := strings.TrimPrefix(file, "packages/bespoke/")
		if filepath.Ext(rel) == ".yaml" && !strings.Contains(rel, "/") {
			impact.bespoke[rel] = struct{}{}
		}
	case strings.HasPrefix(file, "packages/overrides/"):
		rel := strings.TrimPrefix(file, "packages/overrides/")
		if rel != "" && !strings.Contains(rel, "/") {
			impact.overrides[rel] = struct{}{}
		}
	case strings.HasPrefix(file, "packages/pipelines/"):
		rel := strings.TrimPrefix(file, "packages/pipelines/")
		if filepath.Ext(rel) == ".yaml" {
			impact.pipelines[strings.TrimSuffix(rel, ".yaml")] = struct{}{}
		}
	}
	return nil
}

func containsNormalizedPath(files []string, wanted string) bool {
	for _, file := range files {
		if filepath.ToSlash(strings.TrimSpace(file)) == wanted {
			return true
		}
	}
	return false
}

func pathsUnder(files []string, prefix string) bool {
	for _, file := range files {
		if strings.HasPrefix(filepath.ToSlash(strings.TrimSpace(file)), prefix) {
			return true
		}
	}
	return false
}
