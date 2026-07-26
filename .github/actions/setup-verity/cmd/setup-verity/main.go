package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "setup-verity: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	if len(arguments) == 0 {
		return errMissingCommand
	}
	switch arguments[0] {
	case "build":
		return runBuild(ctx, arguments[1:])
	case "verify-remote":
		return runVerifyRemote(ctx, arguments[1:])
	case "extract":
		return runExtract(arguments[1:])
	case "activate":
		return runActivate(arguments[1:])
	default:
		return errUnsupportedCommand
	}
}

func runBuild(ctx context.Context, arguments []string) error {
	flags := newFlagSet("build")
	root := flags.String("root", ".", "repository root")
	sourceSHA := flags.String("source-sha", "", "exact source commit")
	runID := flags.Int64("run-id", 0, "GitHub run ID")
	runAttempt := flags.Int64("run-attempt", 0, "GitHub run attempt")
	artifactDirectory := flags.String("artifact-directory", "", "artifact output directory")
	githubOutput := flags.String("github-output", "", "GitHub output file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	_, err := buildArtifact(ctx, &buildOptions{
		Root: *root, ArtifactDirectory: *artifactDirectory, SourceSHA: *sourceSHA,
		RunID: *runID, RunAttempt: *runAttempt, GitHubOutput: *githubOutput,
	})
	return err
}

func runVerifyRemote(ctx context.Context, arguments []string) error {
	flags := newFlagSet("verify-remote")
	artifactName := flags.String("artifact-name", "", "exact artifact name")
	artifactDigest := flags.String("artifact-digest", "", "exact artifact digest")
	sourceSHA := flags.String("source-sha", "", "exact source commit")
	buildKey := flags.String("build-key", "", "exact build key")
	repository := flags.String("repository", "", "GitHub repository")
	runID := flags.Int64("run-id", 0, "GitHub run ID")
	runAttempt := flags.Int64("run-attempt", 0, "GitHub run attempt")
	protectedProducer := flags.String("protected-producer", "false", "protected producer identity mode")
	protected := flags.String("protected-attestation", "false", "protected attestation mode")
	githubOutput := flags.String("github-output", "", "GitHub output file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	protectedValue, err := parseStrictBoolean(*protected)
	if err != nil {
		return err
	}
	protectedProducerValue, err := parseStrictBoolean(*protectedProducer)
	if err != nil {
		return err
	}
	return verifyRemoteArtifact(ctx, &remoteOptions{
		APIBaseURL: os.Getenv("GITHUB_API_URL"), Token: os.Getenv("GH_TOKEN"), Repository: *repository,
		RunID: *runID, RunAttempt: *runAttempt, ArtifactName: *artifactName, ArtifactDigest: *artifactDigest,
		Identity:          artifactIdentity{SourceSHA: *sourceSHA, BuildKey: *buildKey},
		ProtectedProducer: protectedProducerValue, ProtectedAttestation: protectedValue,
		GitHubOutput: *githubOutput,
	})
}

func parseStrictBoolean(value string) (bool, error) {
	switch value {
	case "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, untrusted("protected attestation input")
	}
}

func runExtract(arguments []string) error {
	flags := newFlagSet("extract")
	downloadDirectory := flags.String("download-directory", "", "download directory")
	artifactDirectory := flags.String("artifact-directory", "", "artifact directory")
	artifactDigest := flags.String("artifact-digest", "", "exact artifact digest")
	sourceSHA := flags.String("source-sha", "", "exact source commit")
	buildKey := flags.String("build-key", "", "exact build key")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	_, err := extractArtifact(&extractOptions{
		DownloadDirectory: *downloadDirectory, ArtifactDirectory: *artifactDirectory, ArtifactDigest: *artifactDigest,
		Identity: artifactIdentity{SourceSHA: *sourceSHA, BuildKey: *buildKey},
	})
	return err
}

func runActivate(arguments []string) error {
	flags := newFlagSet("activate")
	artifactDirectory := flags.String("artifact-directory", "", "artifact directory")
	sourceSHA := flags.String("source-sha", "", "exact source commit")
	buildKey := flags.String("build-key", "", "exact build key")
	destination := flags.String("destination", "", "verified binary destination")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	return activateArtifact(activationOptions{
		ArtifactDirectory: *artifactDirectory, Destination: *destination,
		Identity: artifactIdentity{SourceSHA: *sourceSHA, BuildKey: *buildKey},
	})
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}
