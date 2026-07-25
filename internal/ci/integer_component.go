package ci

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func StageIntegerComponent(ctx context.Context, options *IntegerComponentOptions) (IntegerComponentManifest, error) {
	if err := ctx.Err(); err != nil {
		return IntegerComponentManifest{}, fmt.Errorf("stage Integer component: %w", err)
	}
	stage, err := prepareIntegerComponentStage(ctx, options)
	if err != nil {
		return IntegerComponentManifest{}, err
	}
	return stage.write(ctx)
}

type integerComponentStage struct {
	options  *IntegerComponentOptions
	target   *IntegerBatchTarget
	packages []inspectedIntegerPackage
}

func prepareIntegerComponentStage(ctx context.Context, options *IntegerComponentOptions) (*integerComponentStage, error) {
	if options == nil || options.Plan == nil || options.TargetID == "" || options.PackagesDir == "" || options.OutputDir == "" {
		return nil, fmt.Errorf("%w: component options are incomplete", ErrIntegerBatchPlan)
	}
	if err := validateIntegerBatchPlan(options.Plan); err != nil {
		return nil, err
	}
	target, ok := findIntegerTarget(options.Plan.Targets, options.TargetID)
	if !ok {
		return nil, fmt.Errorf("%w: unknown target %s", ErrIntegerBatchPlan, options.TargetID)
	}
	packages, err := inspectIntegerPackageDirectory(ctx, options.PackagesDir)
	if err != nil {
		return nil, err
	}
	if err := validateIntegerComponentPackages(target, packages); err != nil {
		return nil, err
	}
	return &integerComponentStage{options: options, target: target, packages: packages}, nil
}

func (stage *integerComponentStage) write(ctx context.Context) (IntegerComponentManifest, error) {
	if err := os.RemoveAll(stage.options.OutputDir); err != nil {
		return IntegerComponentManifest{}, fmt.Errorf("clear component output: %w", err)
	}
	if err := os.MkdirAll(stage.options.OutputDir, 0o755); err != nil {
		return IntegerComponentManifest{}, fmt.Errorf("create component output: %w", err)
	}
	manifest := IntegerComponentManifest{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     stage.options.Plan.SourceSHA,
		RunID:         stage.options.Plan.RunID,
		RunAttempt:    stage.options.Plan.RunAttempt,
		PublicationID: stage.options.Plan.PublicationID,
		BatchID:       stage.options.Plan.BatchID,
		Mode:          stage.options.Plan.Mode,
		Event:         stage.options.Plan.Event,
		TargetID:      stage.target.ID(),
		Shard:         stage.target.Shard,
	}
	for _, pkg := range stage.packages {
		if !containsString(stage.target.PublishPackages, pkg.identity.name) {
			continue
		}
		relative := filepath.ToSlash(filepath.Join("packages", string(pkg.identity.architecture), filepath.Base(pkg.path)))
		if err := copyIntegerFile(ctx, pkg.path, filepath.Join(stage.options.OutputDir, filepath.FromSlash(relative))); err != nil {
			return IntegerComponentManifest{}, err
		}
		manifest.Packages = append(manifest.Packages, IntegerPackageFile{
			Architecture: pkg.identity.architecture,
			Name:         pkg.identity.name,
			SHA256:       pkg.identity.digest,
			Path:         relative,
		})
	}
	sortIntegerPackageFiles(manifest.Packages)
	data, err := MarshalIntegerComponentManifest(&manifest)
	if err != nil {
		return IntegerComponentManifest{}, err
	}
	if err := os.WriteFile(filepath.Join(stage.options.OutputDir, IntegerComponentManifestName), data, 0o600); err != nil {
		return IntegerComponentManifest{}, fmt.Errorf("write Integer component manifest: %w", err)
	}
	return manifest, nil
}

type inspectedIntegerPackage struct {
	path     string
	identity integerAPKIdentity
}

func inspectIntegerPackageDirectory(ctx context.Context, root string) ([]inspectedIntegerPackage, error) {
	packages := make([]inspectedIntegerPackage, 0)
	seen := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".apk" {
			return nil
		}
		identity, err := inspectIntegerAPK(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("make APK path relative: %w", err)
		}
		parentArchitecture := IntegerArchitecture(strings.Split(filepath.ToSlash(relative), "/")[0])
		if identity.architecture != parentArchitecture {
			return fmt.Errorf("%w: %s declares %s under %s", ErrIntegerPackageArchitecture, path, identity.architecture, parentArchitecture)
		}
		key := string(identity.architecture) + "/" + identity.name
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: %s", ErrIntegerPackageDuplicate, key)
		}
		seen[key] = struct{}{}
		packages = append(packages, inspectedIntegerPackage{path: path, identity: identity})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inspect Integer packages: %w", err)
	}
	sort.Slice(packages, func(i, j int) bool {
		left := string(packages[i].identity.architecture) + "/" + packages[i].identity.name
		right := string(packages[j].identity.architecture) + "/" + packages[j].identity.name
		return left < right
	})
	return packages, nil
}

func validateIntegerComponentPackages(target *IntegerBatchTarget, packages []inspectedIntegerPackage) error {
	found := map[string]struct{}{}
	for _, pkg := range packages {
		if !containsString(target.ExpectedPackages, pkg.identity.name) {
			return fmt.Errorf("%w: %s/%s", ErrIntegerPackageUndeclared, pkg.identity.architecture, pkg.identity.name)
		}
		found[string(pkg.identity.architecture)+"/"+pkg.identity.name] = struct{}{}
	}
	for _, architecture := range []IntegerArchitecture{IntegerArchitectureX8664, IntegerArchitectureAArch64} {
		for _, name := range target.ExpectedPackages {
			key := string(architecture) + "/" + name
			if _, exists := found[key]; !exists {
				return fmt.Errorf("%w: %s", ErrIntegerPackageMissing, key)
			}
		}
	}
	return nil
}

func copyIntegerFile(ctx context.Context, source, destination string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create package directory: %w", err)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open package source: %w", err)
	}
	defer func() { err = errorsJoin(err, input.Close()) }()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create package destination: %w", err)
	}
	defer func() { err = errorsJoin(err, output.Close()) }()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy package: %w", err)
	}
	return nil
}

func findIntegerTarget(targets []IntegerBatchTarget, id string) (*IntegerBatchTarget, bool) {
	for index := range targets {
		if targets[index].ID() == id {
			return &targets[index], true
		}
	}
	return nil, false
}
