package cmd

import (
	"context"
	"errors"
	"fmt"

	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/melange"
)

const (
	integerMelangeRepoDir = "packages/repo"
	integerMelangeKeyPath = "melange-work/melange.rsa.pub"
)

var errIntegerMelangeArtifactsMissing = errors.New("bespoke package build did not produce repository index and public key")

func integerPrepareMelangeBuild(ctx context.Context, configSpec *intconfig.MelangeSpec, version, arch string) (repos, keyrings []string, err error) {
	if configSpec == nil {
		return nil, nil, nil
	}

	spec, err := melange.ResolveConfigSpec(configSpec, version)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve bespoke package: %w", err)
	}
	architecture, err := melange.ParseArchitecture(arch)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve bespoke package architecture: %w", err)
	}
	paths := melange.DefaultPaths(".")
	if melange.ArtifactsExist(&paths, spec, architecture) {
		return []string{integerMelangeRepoDir}, []string{integerMelangeKeyPath}, nil
	}

	if err := integerMelangeBuild(ctx, &melange.BuildOptions{
		Paths: paths,
		Spec:  spec,
		Arch:  architecture,
	}); err != nil {
		return nil, nil, err
	}
	if !melange.ArtifactsExist(&paths, spec, architecture) {
		return nil, nil, errIntegerMelangeArtifactsMissing
	}

	return []string{integerMelangeRepoDir}, []string{integerMelangeKeyPath}, nil
}
