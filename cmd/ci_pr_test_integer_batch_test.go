package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakePRIntegerBatchRunner struct {
	calls []prCommandRequest
}

func (r *fakePRIntegerBatchRunner) Run(_ context.Context, request *prCommandRequest) (prCommandResult, error) {
	r.calls = append(r.calls, *request)
	if request.Name == "docker" && len(request.Args) > 0 {
		switch request.Args[0] {
		case "load":
			return prCommandResult{Stdout: []byte("Loaded image: local/sealed-secrets:test\n")}, nil
		case "image":
			return prCommandResult{Stdout: []byte(`[{"Id":"sha256:` + strings.Repeat("c", 64) + `","Architecture":"amd64","Config":{"User":"65532"}}]`)}, nil
		}
	}
	if request.Name == "trivy" {
		path := argumentAfter(request.Args, "--output")
		if path != "" {
			if err := os.WriteFile(path, []byte(`{"Results":[]}`), 0o600); err != nil {
				return prCommandResult{}, err
			}
		}
	}
	if strings.HasSuffix(request.Name, "verity") && containsArguments(request.Args, "integer", "build") {
		path := argumentAfter(request.Args, "--output")
		if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
			return prCommandResult{}, err
		}
	}
	return prCommandResult{}, nil
}

type fakePRNativeChecks struct {
	packageKind string
	image       string
}

func (f *fakePRNativeChecks) TestPackage(_ context.Context, request prNativePackageCheck) error {
	f.packageKind = request.Kind
	return nil
}

func (f *fakePRNativeChecks) VerifySealedSecretsImage(_ context.Context, request prSealedSecretsCheck) error {
	f.image = request.Image
	return nil
}

func TestExecutePRIntegerEntry_keeps_native_checks_and_strict_zero_CVE_scan(t *testing.T) {
	// Given: a native Sealed Secrets smoke entry and fake external tools.
	root := t.TempDir()
	spec := filepath.Join(root, "melange-work", "specs", "sealed-secrets-0.yaml", "build.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(spec), 0o755))
	require.NoError(t, os.WriteFile(spec, []byte("package:\n  name: sealed-secrets-0\n  version: 0.38.4\n  epoch: 0\n"), 0o600))
	runner := &fakePRIntegerBatchRunner{}
	native := &fakePRNativeChecks{}
	request := prIntegerBatchRequest{
		Kind: prIntegerBatchSmoke, Architecture: "amd64", PackageArchitecture: "x86_64",
		RepoRoot: root, RunnerTemp: t.TempDir(), SecurityDir: filepath.Join(root, "security"),
		ReportsDir: filepath.Join(root, "reports"), VerityPath: filepath.Join(root, "verity"),
	}
	require.NoError(t, os.MkdirAll(request.SecurityDir, 0o755))
	require.NoError(t, os.MkdirAll(request.ReportsDir, 0o755))

	// When: the typed batch executor runs the entry.
	err := executePRIntegerEntry(t.Context(), &prIntegerDependencies{
		Commands: runner, Native: native, Stdout: io.Discard, Stderr: io.Discard,
	}, &request, prIntegerEntry{Image: "sealed-secrets", Version: "0", Type: "default"})

	// Then: native package/image checks use the immutable loaded ID and Trivy scans every severity.
	require.NoError(t, err)
	require.Equal(t, "sealed-secrets", native.packageKind)
	require.Equal(t, "sha256:"+strings.Repeat("c", 64), native.image)
	require.FileExists(t, filepath.Join(request.SecurityDir, "smoke-sealed-secrets-0-default-amd64.passed"))
	var trivyArgs []string
	for _, call := range runner.calls {
		if call.Name == "trivy" {
			trivyArgs = call.Args
		}
	}
	require.Contains(t, trivyArgs, "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL")
	require.Contains(t, trivyArgs, "--exit-code")
	require.Contains(t, trivyArgs, "1")
}

func TestRunPRIntegerPackageChecks_routes_native_special_cases(t *testing.T) {
	for _, kind := range []string{"rclone", "sealed-secrets", "step-ca"} {
		t.Run(kind, func(t *testing.T) {
			// Given: a smoke entry with a package-specific native test.
			native := &fakePRNativeChecks{}

			// When: package checks are dispatched.
			request := &prIntegerBatchRequest{Kind: prIntegerBatchSmoke, PackageArchitecture: "aarch64"}
			err := runPRIntegerPackageChecks(t.Context(), &prIntegerDependencies{
				Native: native,
			}, request, prIntegerEntry{Image: kind, Version: "1", Type: "default"})

			// Then: the exact typed native check is retained.
			require.NoError(t, err)
			require.Equal(t, kind, native.packageKind)
		})
	}
}

func argumentAfter(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func containsArguments(args []string, want ...string) bool {
	if len(args) < len(want) {
		return false
	}
	for start := 0; start <= len(args)-len(want); start++ {
		matched := true
		for offset := range want {
			matched = matched && args[start+offset] == want[offset]
		}
		if matched {
			return true
		}
	}
	return false
}
