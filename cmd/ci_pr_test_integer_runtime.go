package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func runPRIntegerRuntimeChecks(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	entry prIntegerEntry,
	loaded prIntegerLoadedImage,
) error {
	switch request.Kind {
	case prIntegerBatchSmoke:
		switch entry.Image {
		case "rclone":
			metadata, err := readPRPackageMetadata(request.RepoRoot, "rclone.yaml")
			if err != nil {
				return err
			}
			return runPRRcloneSmoke(ctx, deps, request, loaded.Reference, metadata)
		case "pushgateway":
			metadata, err := readPRPackageMetadata(request.RepoRoot, "pushgateway.yaml")
			if err != nil {
				return err
			}
			if loaded.User != "65532" {
				return fmt.Errorf("%w: Pushgateway runtime user is %q, want 65532", errPRCommandFailed, loaded.User)
			}
			return runPRPushgatewaySmoke(ctx, deps, request, loaded.Reference, metadata)
		case "weaviate":
			metadata, err := readPRPackageMetadata(request.RepoRoot, "weaviate.yaml")
			if err != nil {
				return err
			}
			if loaded.User != "65532" {
				return fmt.Errorf("%w: Weaviate runtime user is %q, want 65532", errPRCommandFailed, loaded.User)
			}
			return runPRWeaviateSmoke(ctx, deps, request, loaded.Reference, metadata)
		case "sealed-secrets":
			metadata, err := readPRPackageMetadata(request.RepoRoot, "sealed-secrets-0.yaml")
			if err != nil {
				return err
			}
			return deps.Native.VerifySealedSecretsImage(ctx, prSealedSecretsCheck{
				Image: loaded.Reference, Version: metadata.Version,
				FullVersion: metadata.FullVersion, TempDir: request.RunnerTemp,
			})
		}
	case prIntegerBatchBuild:
		if entry.Image == "pushgateway" {
			metadata, err := readPRPackageMetadata(request.RepoRoot, "pushgateway.yaml")
			if err != nil {
				return err
			}
			return runPRPushgatewaySmoke(ctx, deps, request, loaded.Reference, metadata)
		}
	}
	return nil
}

func removePRIntegerContainer(ctx context.Context, deps *prIntegerDependencies, name string) error {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, err := deps.Commands.Run(cleanupContext, &prCommandRequest{
		Name: "docker", Args: []string{"rm", "--force", name},
	})
	if err != nil {
		return fmt.Errorf("remove runtime container %s: %w", name, err)
	}
	return nil
}

func joinPRCleanup(err error, cleanup func() error) error {
	return errors.Join(err, cleanup())
}
