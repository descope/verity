package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/urfave/cli/v3"

	repositoryops "github.com/verity-org/verity/internal/ci/repositoryops"
)

var prArtifactUnsafePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type prCopaMetadataInput struct {
	ConfigPath      string
	ImageName       string
	ImageTag        string
	Platform        string
	StagingRegistry string
}

type prCopaMetadata struct {
	Source       string
	GoVCSURL     string
	ArtifactName string
	Destination  string
}

func newCIPrCopaMetadataCommand() *cli.Command {
	return &cli.Command{
		Name:  "copa-metadata",
		Usage: "Resolve typed Copa catalog and staging metadata",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Value: "copa-config.yaml"},
			&cli.StringFlag{Name: "image-name", Required: true},
			&cli.StringFlag{Name: "image-tag", Required: true},
			&cli.StringFlag{Name: "platform", Required: true},
			&cli.StringFlag{Name: "staging-registry", Required: true},
			&cli.StringFlag{Name: "github-output", Required: true},
		},
		Action: runCIPrCopaMetadata,
	}
}

func newCIPrCopaPinCommand() *cli.Command {
	return &cli.Command{
		Name:  "copa-pin",
		Usage: "Compose and validate a digest-bound patched image reference",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "destination", Required: true},
			&cli.StringFlag{Name: "digest", Required: true},
			&cli.StringFlag{Name: "github-output", Required: true},
		},
		Action: func(_ context.Context, command *cli.Command) error {
			image, err := pinnedPRCopaImage(command.String("destination"), command.String("digest"))
			if err != nil {
				return err
			}
			return appendPRGitHubValues(command.String("github-output"), [][2]string{{"image", image}})
		},
	}
}

func runCIPrCopaMetadata(_ context.Context, command *cli.Command) error {
	metadata, err := resolvePRCopaMetadata(&prCopaMetadataInput{
		ConfigPath: command.String("config"), ImageName: command.String("image-name"),
		ImageTag: command.String("image-tag"), Platform: command.String("platform"),
		StagingRegistry: command.String("staging-registry"),
	})
	if err != nil {
		return err
	}
	if err := appendPRGitHubValues(command.String("github-output"), [][2]string{
		{"source", metadata.Source},
		{"go_vcs_url", metadata.GoVCSURL},
		{"artifact_name", metadata.ArtifactName},
		{"destination", metadata.Destination},
	}); err != nil {
		return err
	}
	_, err = fmt.Fprintf(command.Writer, "Copa source: %s -> %s\n", metadata.Source, metadata.Destination)
	if err != nil {
		return fmt.Errorf("write Copa metadata summary: %w", err)
	}
	return nil
}

func resolvePRCopaMetadata(input *prCopaMetadataInput) (prCopaMetadata, error) {
	request, err := repositoryops.NewCatalogRequest(repositoryops.CatalogRequestInput{
		ConfigPath: input.ConfigPath, ImageName: input.ImageName, ImageTag: input.ImageTag,
	})
	if err != nil {
		return prCopaMetadata{}, err
	}
	entry, err := repositoryops.ReadCatalogEntry(request)
	if err != nil {
		return prCopaMetadata{}, err
	}
	artifactName := prArtifactUnsafePattern.ReplaceAllString(strings.TrimSpace(input.ImageName), "-")
	if len(artifactName) > 80 {
		artifactName = artifactName[:80]
	}
	if artifactName == "" {
		return prCopaMetadata{}, fmt.Errorf("%w: artifact name is empty", errPRCommandFailed)
	}
	destination, err := prCopaDestination(input, entry.Source)
	if err != nil {
		return prCopaMetadata{}, err
	}
	return prCopaMetadata{
		Source: entry.Source, GoVCSURL: entry.GoVCSURL,
		ArtifactName: artifactName, Destination: destination,
	}, nil
}

func prCopaDestination(input *prCopaMetadataInput, source string) (string, error) {
	platform := strings.TrimSpace(input.Platform)
	if platform != "linux/amd64" && platform != "linux/arm64" {
		return "", fmt.Errorf("%w: unsupported Copa platform %q", errPRCommandFailed, input.Platform)
	}
	staging := strings.TrimSpace(input.StagingRegistry)
	if _, err := name.NewRepository(staging, name.StrictValidation); err != nil {
		return "", fmt.Errorf("%w: invalid staging registry: %w", errPRCommandFailed, err)
	}
	withoutDigest, _, _ := strings.Cut(source, "@")
	lastSlash := strings.LastIndex(withoutDigest, "/")
	lastColon := strings.LastIndex(withoutDigest, ":")
	if lastColon <= lastSlash || lastColon == len(withoutDigest)-1 {
		return "", fmt.Errorf("%w: source image must include a tag", errPRCommandFailed)
	}
	sourceTag := withoutDigest[lastColon+1:]
	safeImage := strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(strings.TrimSpace(input.ImageName))
	destination := staging + ":" + safeImage + "-" + sourceTag + "-" + strings.TrimPrefix(platform, "linux/")
	if _, err := name.NewTag(destination, name.StrictValidation); err != nil {
		return "", fmt.Errorf("%w: invalid Copa destination: %w", errPRCommandFailed, err)
	}
	return destination, nil
}

func pinnedPRCopaImage(destination, digest string) (string, error) {
	if !prSHA256DigestPattern.MatchString(strings.TrimSpace(digest)) {
		return "", fmt.Errorf("%w: malformed Copa staging digest %q", errPRCommandFailed, digest)
	}
	reference := strings.TrimSpace(destination) + "@" + strings.TrimSpace(digest)
	if _, err := name.ParseReference(reference, name.StrictValidation); err != nil {
		return "", fmt.Errorf("%w: invalid pinned Copa image: %w", errPRCommandFailed, err)
	}
	return reference, nil
}
