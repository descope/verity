package melange

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
)

const (
	RepositoryURL = "https://packages.wolfi.dev/os"
	KeyringURL    = "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"
)

type Paths struct {
	Root         string
	ImagesDir    string
	LockFile     string
	LockedDir    string
	BespokeDir   string
	PipelinesDir string
	OverridesDir string
	WorkDir      string
	RepoDir      string
}

func DefaultPaths(root string) Paths {
	root = filepath.Clean(root)
	return Paths{
		Root:         root,
		ImagesDir:    filepath.Join(root, "images"),
		LockFile:     filepath.Join(root, "packages", "upstream.lock.json"),
		LockedDir:    filepath.Join(root, "packages", "bespoke", "locked"),
		BespokeDir:   filepath.Join(root, "packages", "bespoke"),
		PipelinesDir: filepath.Join(root, "packages", "pipelines"),
		OverridesDir: filepath.Join(root, "packages", "overrides"),
		WorkDir:      filepath.Join(root, "melange-work"),
		RepoDir:      filepath.Join(root, "packages", "repo"),
	}
}

type Spec struct {
	Upstream    string
	Bespoke     []string
	EnvFile     string
	BuildOption string
}

func (s Spec) Needed() bool {
	return s.Upstream != "" || len(s.Bespoke) > 0
}

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

type Runner interface {
	Run(context.Context, *Command, io.Writer, io.Writer) error
}

type BuildOptions struct {
	Paths  Paths
	Spec   Spec
	Arch   Architecture
	Staged bool
	Runner Runner
	Stdout io.Writer
	Stderr io.Writer
}

type Architecture string

const (
	ArchitectureX8664   Architecture = "x86_64"
	ArchitectureAArch64 Architecture = "aarch64"
)

func ParseArchitecture(value string) (Architecture, error) {
	switch value {
	case "amd64", string(ArchitectureX8664):
		return ArchitectureX8664, nil
	case "arm64", string(ArchitectureAArch64):
		return ArchitectureAArch64, nil
	default:
		return "", fmt.Errorf("%w %q", errUnsupportedArchitecture, value)
	}
}

func (a Architecture) valid() bool {
	return a == ArchitectureX8664 || a == ArchitectureAArch64
}
