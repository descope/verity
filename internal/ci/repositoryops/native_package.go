package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const nativeTestTimeout = 30 * time.Minute

var (
	ErrUnsupportedArchitecture = errors.New("unsupported package architecture")
	ErrUnsupportedNativeTest   = errors.New("unsupported native package test")
)

type NativePackageInput struct {
	Kind           string
	Architecture   string
	RepositoryRoot string
}

type NativePackageRequest struct {
	kind         string
	architecture string
	repoRoot     string
	buildFile    string
	packageName  string
}

func NewNativePackageRequest(input NativePackageInput) (NativePackageRequest, error) {
	architecture := strings.TrimSpace(input.Architecture)
	if architecture != "x86_64" && architecture != "aarch64" {
		return NativePackageRequest{}, fmt.Errorf("%w: %q", ErrUnsupportedArchitecture, input.Architecture)
	}
	root, err := validatedPath("repository root", input.RepositoryRoot)
	if err != nil {
		return NativePackageRequest{}, err
	}
	kind := strings.TrimSpace(input.Kind)
	request := NativePackageRequest{kind: kind, architecture: architecture, repoRoot: filepath.Clean(root)}
	switch kind {
	case "rclone":
		request.buildFile = "melange-work/specs/rclone.yaml/build.yaml"
		request.packageName = "rclone"
	case "sealed-secrets":
		request.buildFile = "melange-work/specs/sealed-secrets-0.yaml/build.yaml"
		request.packageName = "sealed-secrets-0"
	case "step-ca":
		request.buildFile = "melange-work/specs/step-ca.yaml/build.yaml"
		request.packageName = "step-ca"
	default:
		return NativePackageRequest{}, fmt.Errorf("%w: %q", ErrUnsupportedNativeTest, input.Kind)
	}
	return request, nil
}

type NativeService struct {
	Commands CommandRunner
}

func (s NativeService) TestPackage(ctx context.Context, request *NativePackageRequest) error {
	if request == nil {
		return fmt.Errorf("%w: package request is required", ErrUnsupportedNativeTest)
	}
	commands := s.Commands
	if commands == nil {
		commands = ExecCommandRunner{}
	}
	testContext, cancel := context.WithTimeout(ctx, nativeTestTimeout)
	defer cancel()
	_, err := runRequiredCommand(testContext, commands, &Command{
		Name: "melange",
		Dir:  request.repoRoot,
		Args: []string{
			"test", "--arch", request.architecture,
			"--repository-append", filepath.Join(request.repoRoot, "packages", "repo"),
			"--repository-append", "https://packages.wolfi.dev/os",
			"--keyring-append", filepath.Join(
				request.repoRoot,
				"packages",
				"repo",
				"melange-"+request.architecture+".rsa.pub",
			),
			"--keyring-append", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub",
			"--runner", "docker", "--pipeline-dirs", "melange-work/pipelines",
			request.buildFile, request.packageName,
		},
	})
	if err != nil {
		return fmt.Errorf("test %s package for %s: %w", request.kind, request.architecture, err)
	}
	return nil
}
