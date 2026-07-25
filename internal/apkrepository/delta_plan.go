package apkrepository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type plannedMutation struct {
	operation DeltaOperation
	candidate *inspectedPackage
	previous  []inspectedPackage
	changed   bool
}

type deltaPlan struct {
	mutations      []plannedMutation
	affectedArches []string
	changed        int
	unchanged      int
}

func buildDeltaPlan(config *deltaConfig, manifest *DeltaManifest, base *deltaBase) (*deltaPlan, error) {
	plan := &deltaPlan{mutations: make([]plannedMutation, 0, len(manifest.Operations))}
	affected := make(map[string]struct{})
	for _, operation := range manifest.Operations {
		key := packageKey{architecture: operation.Architecture, name: operation.PackageName}
		mutation := plannedMutation{operation: operation, previous: base.packages[key]}
		switch operation.Action {
		case DeltaActionRemove:
			if len(mutation.previous) == 0 {
				return nil, fmt.Errorf("%w: %s/%s", errDeltaPackageMissing, key.architecture, key.name)
			}
			mutation.changed = true
		case DeltaActionUpsert:
			candidate, err := inspectPackage(filepath.Join(config.packageDir, filepath.FromSlash(operation.Source)))
			if err != nil {
				return nil, err
			}
			if candidate.key != key {
				return nil, fmt.Errorf("%w: declared %s/%s, APK is %s/%s", errDeltaPackageMismatch, key.architecture, key.name, candidate.key.architecture, candidate.key.name)
			}
			if candidate.digest != operation.SHA256 {
				return nil, fmt.Errorf("%w for %s/%s: declared %s, APK is %s", errDeltaDigestMismatch, key.architecture, key.name, operation.SHA256, candidate.digest)
			}
			mutation.candidate = &candidate
			mutation.changed = !containsSemanticPackage(mutation.previous, candidate.digest)
		}
		if mutation.changed {
			plan.changed++
			affected[operation.Architecture] = struct{}{}
		} else {
			plan.unchanged++
		}
		plan.mutations = append(plan.mutations, mutation)
	}
	plan.affectedArches = make([]string, 0, len(affected))
	for architecture := range affected {
		plan.affectedArches = append(plan.affectedArches, architecture)
	}
	sort.Strings(plan.affectedArches)
	return plan, nil
}

func containsSemanticPackage(packages []inspectedPackage, digest string) bool {
	for _, pkg := range packages {
		if pkg.digest == digest {
			return true
		}
	}
	return false
}

type deltaExecution struct {
	ctx        context.Context
	config     *deltaConfig
	plan       *deltaPlan
	stage      string
	privateKey string
}

func (execution *deltaExecution) apply() error {
	if err := execution.removePreviousPackages(); err != nil {
		return err
	}
	if err := execution.installUpserts(); err != nil {
		return err
	}
	return execution.reindexAffectedArchitectures()
}

func (execution *deltaExecution) removePreviousPackages() error {
	for _, mutation := range execution.plan.mutations {
		if !mutation.changed {
			continue
		}
		for _, previous := range mutation.previous {
			relative, err := filepath.Rel(execution.config.baseDir, previous.path)
			if err != nil {
				return fmt.Errorf("resolve prior package %q: %w", previous.path, err)
			}
			if err := removeIfExists(filepath.Join(execution.stage, relative)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (execution *deltaExecution) installUpserts() error {
	for _, mutation := range execution.plan.mutations {
		if !mutation.changed || mutation.candidate == nil {
			continue
		}
		destination := filepath.Join(execution.stage, mutation.operation.Architecture, mutation.candidate.fileName)
		if err := rejectPackageCollision(destination, mutation.candidate.key); err != nil {
			return err
		}
		if err := copyFile(mutation.candidate.path, destination); err != nil {
			return err
		}
		if _, err := runRequired(execution.ctx, execution.config.runner, &command{
			name: "melange", args: []string{"sign", "--signing-key", execution.privateKey, destination},
		}); err != nil {
			return fmt.Errorf("sign package %s: %w", mutation.candidate.path, err)
		}
		signed, err := inspectPackage(destination)
		if err != nil {
			return err
		}
		if signed.digest != mutation.candidate.digest {
			return fmt.Errorf("%w after signing %s", errDeltaDigestMismatch, mutation.candidate.path)
		}
	}
	return nil
}

func (execution *deltaExecution) reindexAffectedArchitectures() error {
	packages, err := inspectRepositoryPackages(execution.stage)
	if err != nil {
		return err
	}
	if err := requireDualArchitecturePackages(packages); err != nil {
		return err
	}
	indexConfig := &assembleConfig{
		outputDir: execution.stage, keyName: execution.config.keyName,
		stdout: execution.config.stdout, stderr: execution.config.stderr, runner: execution.config.runner,
	}
	return createIndexesForArchitectures(execution.ctx, indexConfig, execution.privateKey, execution.plan.affectedArches)
}

func rejectPackageCollision(path string, wanted packageKey) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat delta destination %q: %w", path, err)
	}
	existing, err := inspectPackage(path)
	if err != nil {
		return err
	}
	if existing.key != wanted {
		return fmt.Errorf("%w %s: existing package is %s/%s", errDuplicateDestination, path, existing.key.architecture, existing.key.name)
	}
	return nil
}
