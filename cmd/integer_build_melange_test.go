package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/melange"
)

func TestIntegerPrepareMelangeBuild_ResolvesVersionAndBuildsLocally(t *testing.T) {
	// Given: a versioned recipe and a Go build boundary that produces the expected artifacts.
	repoRoot := t.TempDir()
	chdirIntegerMelangeTest(t, repoRoot)
	var captured melange.BuildOptions
	originalBuild := integerMelangeBuild
	integerMelangeBuild = func(_ context.Context, options *melange.BuildOptions) error {
		captured = *options
		return writeIntegerMelangeArtifacts(t, options)
	}
	t.Cleanup(func() {
		integerMelangeBuild = originalBuild
	})

	// When: the image build requests the package for a concrete stream and architecture.
	artifacts, err := integerPrepareMelangeBuild(context.Background(), &intconfig.MelangeSpec{
		Upstream:    "cilium-{{version}}",
		EnvFile:     "fips-{{version}}.env",
		BuildOption: "stream-{{version}}",
	}, "1.19", "arm64")

	// Then: placeholders are resolved before the Go build and local repository paths are returned.
	require.NoError(t, err)
	assert.Equal(t, []string{integerMelangeRepository}, artifacts.Repositories)
	assert.Equal(t, []string{"packages/repo/melange-aarch64.rsa.pub"}, artifacts.Keyrings)
	assert.Equal(t, []apkindex.Package{{Name: "cilium-1.19", Version: "1.0-r0"}}, artifacts.Packages)
	assert.Equal(t, melange.Spec{
		Upstream:    "cilium-1.19",
		EnvFile:     "fips-1.19.env",
		BuildOption: "stream-1.19",
	}, captured.Spec)
	assert.Equal(t, melange.ArchitectureAArch64, captured.Arch)
}

func TestIntegerPrepareMelangeBuild_ReusesExistingArtifacts(t *testing.T) {
	// Given: a complete local repository for the requested architecture.
	repoRoot := t.TempDir()
	paths := melange.DefaultPaths(repoRoot)
	spec := melange.Spec{Bespoke: []string{"custom.yaml"}}
	require.NoError(t, writeIntegerMelangeArtifacts(t, &melange.BuildOptions{
		Paths: paths,
		Spec:  spec,
		Arch:  melange.ArchitectureAArch64,
	}))
	require.NoError(t, os.RemoveAll(paths.WorkDir))
	chdirIntegerMelangeTest(t, repoRoot)
	buildCalls := 0
	originalBuild := integerMelangeBuild
	integerMelangeBuild = func(_ context.Context, _ *melange.BuildOptions) error {
		buildCalls++
		return nil
	}
	t.Cleanup(func() {
		integerMelangeBuild = originalBuild
	})

	// When: the same architecture is requested again.
	artifacts, err := integerPrepareMelangeBuild(context.Background(), &intconfig.MelangeSpec{
		Bespoke: intconfig.StringList{"custom.yaml"},
	}, "1.0", "arm64")

	// Then: the existing artifacts are reused without rebuilding.
	require.NoError(t, err)
	assert.Equal(t, []string{integerMelangeRepository}, artifacts.Repositories)
	assert.Equal(t, []string{"packages/repo/melange-aarch64.rsa.pub"}, artifacts.Keyrings)
	assert.Equal(t, []apkindex.Package{{Name: "custom", Version: "1.0-r0"}}, artifacts.Packages)
	assert.Zero(t, buildCalls)
}

type integerMelangeArtifactRunner struct {
	packages []apkindex.Package
}

func (r integerMelangeArtifactRunner) Run(_ context.Context, command *melange.Command, _, _ io.Writer) error {
	switch {
	case len(command.Args) > 0 && command.Args[0] == "keygen":
		writeIntegerMelangeTestFile(command.Dir, command.Args[1], "private")
		writeIntegerMelangeTestFile(command.Dir, command.Args[1]+".pub", "public")
	case len(command.Args) > 0 && command.Args[0] == "build":
		arch := commandArgument(command.Args, "--arch")
		writeIntegerMelangeAPKIndex(command.Dir, filepath.ToSlash(filepath.Join("packages", "repo", arch, "APKINDEX.tar.gz")), r.packages)
		writeIntegerMelangeTestFile(command.Dir, filepath.ToSlash(filepath.Join("packages", "repo", arch, "package.apk")), "package")
	}
	return nil
}

func writeIntegerMelangeArtifacts(t *testing.T, options *melange.BuildOptions) error {
	t.Helper()
	name := "custom"
	if options.Spec.Upstream != "" {
		name = options.Spec.Upstream
	} else if len(options.Spec.Bespoke) > 0 {
		name = strings.TrimSuffix(options.Spec.Bespoke[0], filepath.Ext(options.Spec.Bespoke[0]))
	}
	return writeIntegerMelangeArtifactsWithPackages(t, options, []apkindex.Package{{Name: name, Version: "1.0-r0"}})
}

func writeIntegerMelangeArtifactsWithPackages(t *testing.T, options *melange.BuildOptions, packages []apkindex.Package) error {
	t.Helper()
	if options.Spec.Upstream != "" {
		recipe := "package:\n  name: " + options.Spec.Upstream + "\n"
		writeIntegerMelangeTestFile(options.Paths.Root, "packages/bespoke/locked/"+options.Spec.Upstream+".yaml", recipe)
		lock := fmt.Sprintf(`{"packages":{%q:{"file":%q,"sha256":%q,"assets":{}}},"pipeline_files":{}}`,
			options.Spec.Upstream, options.Spec.Upstream+".yaml", testStringSHA(recipe))
		writeIntegerMelangeTestFile(options.Paths.Root, "packages/upstream.lock.json", lock)
	} else {
		for _, file := range options.Spec.Bespoke {
			name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			writeIntegerMelangeTestFile(options.Paths.Root, filepath.ToSlash(filepath.Join("packages", "bespoke", file)), "package:\n  name: "+name+"\n")
		}
		writeIntegerMelangeTestFile(options.Paths.Root, "packages/upstream.lock.json", `{"packages":{},"pipeline_files":{}}`)
	}
	if options.Spec.EnvFile != "" {
		writeIntegerMelangeTestFile(options.Paths.Root, filepath.ToSlash(filepath.Join("packages", "overrides", options.Spec.EnvFile)), "OPTION=1\n")
	}
	build := *options
	build.Runner = integerMelangeArtifactRunner{packages: packages}
	return melange.Build(context.Background(), &build)
}

func writeIntegerMelangeAPKIndex(root, relative string, packages []apkindex.Package) {
	var body bytes.Buffer
	for _, pkg := range packages {
		fmt.Fprintf(&body, "P:%s\nV:%s\n\n", pkg.Name, pkg.Version)
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	data := body.Bytes()
	if err := tw.WriteHeader(&tar.Header{Name: "APKINDEX", Mode: 0o644, Size: int64(len(data))}); err != nil {
		panic(err)
	}
	if _, err := tw.Write(data); err != nil {
		panic(err)
	}
	if err := tw.Close(); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil {
		panic(err)
	}
	writeIntegerMelangeTestFile(root, relative, archive.String())
}

func writeIntegerMelangeTestFile(root, relative, body string) {
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		panic(err)
	}
}

func commandArgument(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func testStringSHA(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func chdirIntegerMelangeTest(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}

func TestIntegerRunApkoBuild_AppendsExtraRepositoriesAndKeyrings(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "apko")
	body := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > \"" + argsPath + "\"\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	err := integerRunApkoBuild(
		context.Background(),
		"config.yaml",
		"out.tar",
		"amd64",
		[]string{"packages/repo"},
		[]string{"melange-work/melange.rsa.pub"},
	)
	require.NoError(t, err)

	data, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Equal(t, "build\n--arch\namd64\n--repository-append\npackages/repo\n--keyring-append\nmelange-work/melange.rsa.pub\nconfig.yaml\ninteger:local\nout.tar\n", string(data))
}
