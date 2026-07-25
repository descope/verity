package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/verity-org/verity/internal/buildmetadata"
)

type buildOptions struct {
	Root              string
	ArtifactDirectory string
	SourceSHA         string
	RunID             int64
	RunAttempt        int64
	GitHubOutput      string
}

type buildResult struct {
	ArtifactName string
	BuildKey     string
	SourceSHA    string
}

var canonicalBuildEnvironment = []string{
	"GOOS=linux",
	"GOARCH=amd64",
	"GOAMD64=v1",
	"CGO_ENABLED=0",
	"GO111MODULE=on",
	"GOENV=off",
	"GOEXPERIMENT=",
	"GOFIPS140=off",
	"GOFLAGS=",
	"GOTOOLCHAIN=auto",
	"GOWORK=off",
}

func canonicalBuildConfig() buildmetadata.BuildConfig {
	flags := append([]string(nil), canonicalBuildFlags...)
	flags = append(flags, metadataLDFlagsContract)
	return buildmetadata.BuildConfig{GOOS: "linux", GOARCH: "amd64", CGOEnabled: "0", Flags: flags}
}

func buildArtifactName(buildKey string, runID, runAttempt int64) (string, error) {
	if !lowerHex(buildKey, 64) || runID <= 0 || runAttempt <= 0 {
		return "", untrusted("malformed build identity")
	}
	return artifactNamePrefix + buildKey + "-" + strconv.FormatInt(runID, 10) + "-" + strconv.FormatInt(runAttempt, 10), nil
}

func buildArtifact(ctx context.Context, options *buildOptions) (buildResult, error) {
	if !lowerHex(options.SourceSHA, 40) || options.RunID <= 0 || options.RunAttempt <= 0 {
		return buildResult{}, untrusted("malformed build identity")
	}
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return buildResult{}, fmt.Errorf("resolve repository root: %w", err)
	}
	if err := verifySourceTree(ctx, root, options.SourceSHA); err != nil {
		return buildResult{}, err
	}
	config := canonicalBuildConfig()
	buildKey, err := computeProductionBuildKey(ctx, root, config)
	if err != nil {
		return buildResult{}, fmt.Errorf("compute Verity build key: %w", err)
	}
	artifactName, err := buildArtifactName(buildKey, options.RunID, options.RunAttempt)
	if err != nil {
		return buildResult{}, err
	}
	if err := createBuildDirectory(options.ArtifactDirectory); err != nil {
		return buildResult{}, err
	}
	binaryPath := filepath.Join(options.ArtifactDirectory, binaryName)
	if err := compileVerity(ctx, root, binaryPath, artifactIdentity{SourceSHA: options.SourceSHA, BuildKey: buildKey}); err != nil {
		return buildResult{}, err
	}
	if err := packageBuiltBinary(ctx, options.ArtifactDirectory, artifactIdentity{SourceSHA: options.SourceSHA, BuildKey: buildKey}); err != nil {
		return buildResult{}, err
	}
	result := buildResult{ArtifactName: artifactName, BuildKey: buildKey, SourceSHA: options.SourceSHA}
	if options.GitHubOutput != "" {
		if err := appendBuildOutputs(options.GitHubOutput, result); err != nil {
			return buildResult{}, err
		}
	}
	return result, nil
}

func verifySourceTree(ctx context.Context, root, sourceSHA string) error {
	head, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(head) != sourceSHA {
		return untrusted("checked-out source SHA")
	}
	status, err := gitOutput(ctx, root, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("inspect source tree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return untrusted("dirty source tree")
	}
	return nil
}

func gitOutput(ctx context.Context, root string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", arguments[0], err)
	}
	return string(output), nil
}

func createBuildDirectory(directory string) error {
	parent, err := os.Lstat(filepath.Dir(directory))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return untrusted("build artifact parent")
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return fmt.Errorf("create build artifact directory: %w", err)
	}
	return nil
}

func compileVerity(ctx context.Context, root, output string, identity artifactIdentity) error {
	flagsValue := strings.Join(canonicalBuildFlags, " ")
	ldflags := strings.Join([]string{
		"-X", metadataPackage + ".version=" + buildVersion,
		"-X", metadataPackage + ".sourceSHA=" + identity.SourceSHA,
		"-X", metadataPackage + ".buildKey=" + identity.BuildKey,
		"-X", strconv.Quote(metadataPackage + ".buildFlags=" + flagsValue),
	}, " ")
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=true", "-ldflags", ldflags, "-o", output, ".")
	command.Dir = root
	command.Env = buildEnvironment(os.Environ())
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build Verity: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func buildEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+len(canonicalBuildEnvironment))
	for _, value := range environment {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "GOOS", "GOARCH", "GOAMD64", "CGO_ENABLED", "GO111MODULE", "GOENV", "GOEXPERIMENT", "GOFIPS140", "GOFLAGS", "GOTOOLCHAIN", "GOWORK":
			continue
		}
		result = append(result, value)
	}
	return append(result, canonicalBuildEnvironment...)
}

func packageBuiltBinary(ctx context.Context, directory string, identity artifactIdentity) error {
	binaryPath := filepath.Join(directory, binaryName)
	command := exec.CommandContext(ctx, binaryPath, "version", "--json")
	metadataData, err := command.Output()
	if err != nil {
		return fmt.Errorf("read built Verity metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, buildJSONName), metadataData, 0o600); err != nil {
		return fmt.Errorf("write build metadata: %w", err)
	}
	digest, err := digestPath(binaryPath, maxBinarySize)
	if err != nil {
		return fmt.Errorf("digest built Verity: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, checksumName), []byte(digest+"  "+binaryName+"\n"), 0o600); err != nil {
		return fmt.Errorf("write Verity checksum: %w", err)
	}
	if err := os.Chmod(binaryPath, 0o600); err != nil {
		return fmt.Errorf("remove Verity execute permission: %w", err)
	}
	_, err = verifyArtifactDirectory(directory, identity)
	return err
}

func appendBuildOutputs(path string, result buildResult) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open GitHub output: %w", err)
	}
	defer func() { err = errorsJoin(err, file.Close()) }()
	if _, err := fmt.Fprintf(file, "artifact-name=%s\nbuild-key=%s\nsource-sha=%s\n", result.ArtifactName, result.BuildKey, result.SourceSHA); err != nil {
		return fmt.Errorf("write GitHub output: %w", err)
	}
	return nil
}
