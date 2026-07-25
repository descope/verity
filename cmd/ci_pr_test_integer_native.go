package cmd

import (
	"context"

	repositoryops "github.com/verity-org/verity/internal/ci/repositoryops"
)

type repositoryPRIntegerChecks struct{}

func (repositoryPRIntegerChecks) TestPackage(ctx context.Context, check prNativePackageCheck) error {
	request, err := repositoryops.NewNativePackageRequest(repositoryops.NativePackageInput{
		Kind: check.Kind, Architecture: check.Architecture, RepositoryRoot: check.RepoRoot,
	})
	if err != nil {
		return err
	}
	return (repositoryops.NativeService{}).TestPackage(ctx, &request)
}

func (repositoryPRIntegerChecks) VerifySealedSecretsImage(ctx context.Context, check prSealedSecretsCheck) error {
	request, err := repositoryops.NewSealedSecretsImageRequest(repositoryops.SealedSecretsImageInput{
		Image: check.Image, Version: check.Version, FullVersion: check.FullVersion, TempDir: check.TempDir,
	})
	if err != nil {
		return err
	}
	return (repositoryops.NativeService{}).VerifySealedSecretsImage(ctx, request)
}
