package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/melange"
)

const (
	integerMelangeRepoDir    = "packages/repo"
	integerMelangeRepository = "@local " + integerMelangeRepoDir
	integerMelangeKeyPath    = "melange-work/melange.rsa.pub"
	integerMelangePackageTag = "@local"
)

var (
	errIntegerMelangeArtifactsMissing = errors.New("bespoke package build did not produce repository index and public key")
	errIntegerMelangePackageConflict  = errors.New("bespoke package repository contains conflicting versions")
	errIntegerMelangePackageNoVersion = errors.New("bespoke package repository entry has no version")
)

type integerMelangeArtifacts struct {
	Repositories []string
	Keyrings     []string
	Packages     []apkindex.Package
}

func integerPrepareMelangeBuild(ctx context.Context, configSpec *intconfig.MelangeSpec, version, arch string) (integerMelangeArtifacts, error) {
	if configSpec == nil {
		return integerMelangeArtifacts{}, nil
	}

	spec, err := melange.ResolveConfigSpec(configSpec, version)
	if err != nil {
		return integerMelangeArtifacts{}, fmt.Errorf("resolve bespoke package: %w", err)
	}
	architecture, err := melange.ParseArchitecture(arch)
	if err != nil {
		return integerMelangeArtifacts{}, fmt.Errorf("resolve bespoke package architecture: %w", err)
	}
	paths := melange.DefaultPaths(".")
	if !melange.ArtifactsExist(&paths, spec, architecture) {
		if err := integerMelangeBuild(ctx, &melange.BuildOptions{
			Paths: paths,
			Spec:  spec,
			Arch:  architecture,
		}); err != nil {
			return integerMelangeArtifacts{}, err
		}
	}
	if !melange.ArtifactsExist(&paths, spec, architecture) {
		return integerMelangeArtifacts{}, errIntegerMelangeArtifactsMissing
	}

	packages, err := readIntegerMelangePackages(&paths, architecture)
	if err != nil {
		return integerMelangeArtifacts{}, err
	}
	return integerMelangeArtifacts{
		Repositories: []string{integerMelangeRepository},
		Keyrings:     []string{integerMelangeKeyPath},
		Packages:     packages,
	}, nil
}

func readIntegerMelangePackages(paths *melange.Paths, arch melange.Architecture) ([]apkindex.Package, error) {
	indexPath := filepath.Join(paths.RepoDir, string(arch), "APKINDEX.tar.gz")
	index, err := os.Open(indexPath)
	if err != nil {
		return nil, fmt.Errorf("open bespoke package index %q: %w", indexPath, err)
	}
	defer index.Close()

	packages, err := apkindex.ParseArchive(index)
	if err != nil {
		return nil, fmt.Errorf("parse bespoke package index %q: %w", indexPath, err)
	}
	return packages, nil
}

func pinLocalPackageVersions(tmpl *intconfig.TypeTemplate, renderVersion string, packages []apkindex.Package) error {
	versions := make(map[string]string, len(packages))
	for _, pkg := range packages {
		if pkg.Version == "" {
			return fmt.Errorf("%w: %s", errIntegerMelangePackageNoVersion, pkg.Name)
		}
		if previous, exists := versions[pkg.Name]; exists && previous != pkg.Version {
			return fmt.Errorf("%w for %s: %s and %s", errIntegerMelangePackageConflict, pkg.Name, previous, pkg.Version)
		}
		versions[pkg.Name] = pkg.Version
	}

	for index, packageSpec := range tmpl.Packages {
		name := apkPackageName(strings.ReplaceAll(packageSpec, "{{version}}", renderVersion))
		if version, exists := versions[name]; exists {
			tmpl.Packages[index] = name + integerMelangePackageTag + "=" + version
		}
	}
	return nil
}
