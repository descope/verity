package workflowpolicy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type localBuildMutation struct {
	Name        string `yaml:"name"`
	Relative    string `yaml:"relative"`
	Old         string `yaml:"old"`
	Replacement string `yaml:"replacement"`
}

func TestValidateDirectory_rejectsRepositoryWideLocalVerityCompilationMutations(t *testing.T) {
	// Given hostile workflow and composite-action mutations that rebuild Verity locally.
	mutations := readLocalBuildMutations(t)
	for _, mutation := range mutations {
		t.Run(mutation.Name, func(t *testing.T) {
			root := copyLocalBuildPolicyRepository(t)
			path := filepath.Join(root, mutation.Relative)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(data), mutation.Old, "stale mutation fixture")
			require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), mutation.Old, mutation.Replacement, 1)), 0o600))

			// When the complete workflow policy evaluates the mutated repository.
			_, err = ValidateDirectory(filepath.Join(root, ".github", "workflows"))

			// Then a build-once violation identifies the local Verity compilation.
			require.Error(t, err)
			var policyError *PolicyError
			require.True(t, errors.As(err, &policyError))
			found := false
			for _, violation := range policyError.Violations {
				if violation.Rule == RuleBuildOnce && strings.Contains(violation.Detail, "local Verity compilation") {
					found = true
					break
				}
			}
			assert.True(t, found, "missing local compilation violation:\n%s", err)
		})
	}
}

func TestValidateDirectory_allowsNonVerityGoBuildsInWorkflowsAndActions(t *testing.T) {
	// Given workflow and composite-action commands that compile dedicated helper packages,
	// including quoted/env-prefixed commands and builds rooted in helper directories.
	root := copyLocalBuildPolicyRepository(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "integer-build-image-reusable.yaml")
	workflow, err := os.ReadFile(workflowPath)
	require.NoError(t, err)
	workflow = []byte(strings.Replace(string(workflow), "      - name: Prepare bespoke package", `      - name: Build policy helpers
        run: |
          env CGO_ENABLED=0 "go" "build" -o "$RUNNER_TEMP/policy-helper" "./cmd/policy-helper"
          command -v go build
          printf '%s\\n' 'go build -o verity .'

      - name: Build helper from its package directory
        working-directory: ./cmd/package-helper
        run: go build

      - name: Build helper with Go change-directory
        run: go -C ./cmd/change-directory-helper build

      - name: Build helper after shell change-directory
        run: cd ./cmd/helper && go build

      - name: Prepare bespoke package`, 1))
	require.NoError(t, os.WriteFile(workflowPath, workflow, 0o600))

	actionPath := filepath.Join(root, ".github", "actions", "setup-binaries", "action.yml")
	action, err := os.ReadFile(actionPath)
	require.NoError(t, err)
	action = []byte(strings.Replace(string(action), "      run: go mod download", `      run: |
        env --unset=GOFLAGS go build -o "$RUNNER_TEMP/action-helper" ./cmd/action-helper
        go run ./cmd/action-helper`, 1))
	require.NoError(t, os.WriteFile(actionPath, action, 0o600))

	// When the complete workflow policy evaluates the repository.
	_, err = ValidateDirectory(filepath.Join(root, ".github", "workflows"))

	// Then no build-once violation misclassifies the non-Verity builds.
	if err == nil {
		return
	}
	var policyError *PolicyError
	require.True(t, errors.As(err, &policyError))
	for _, violation := range policyError.Violations {
		assert.False(t, violation.Rule == RuleBuildOnce && strings.Contains(violation.Detail, "local Verity compilation"), violation.String())
	}
}

func TestLocalVerityBuildPolicy_classifiesShellVariants_withoutHelperFalsePositives(t *testing.T) {
	tests := []struct {
		name             string
		script           string
		workingDirectory string
		wantViolation    bool
	}{
		{name: "env unset root build", script: "env -u GOFLAGS go build -o ./verity .", wantViolation: true},
		{name: "env workspace chdir root build", script: `env -C "$GITHUB_WORKSPACE" go build`, wantViolation: true},
		{name: "combined shell flags", script: `sh -euc 'go build -o=verity .'`, wantViolation: true},
		{name: "long shell option before command string", script: `bash --norc -c 'go build -o=verity .'`, wantViolation: true},
		{name: "shell option operand before command string", script: `bash --rcfile /dev/null -c 'go build -o=verity .'`, wantViolation: true},
		{name: "command execution wrapper", script: "command -p go build -o verity .", wantViolation: true},
		{name: "exec wrapper", script: "exec go build -o verity .", wantViolation: true},
		{name: "fully quoted tokens", script: `"go" "build" "-o" "./verity" "."`, wantViolation: true},
		{name: "root source glob", script: "go build *.go", wantViolation: true},
		{name: "quoted root source glob token", script: `go build '*.go'`, wantViolation: true},
		{name: "shell change-directory returns to root", script: "cd cmd/helper && cd ../.. && go build", wantViolation: true},
		{name: "verity output from helper package", script: `go build -o "$RUNNER_TEMP/verity" ./cmd/helper`, wantViolation: true},
		{name: "root module install", script: "go install github.com/verity-org/verity@latest", wantViolation: true},
		{name: "command query", script: "command -v go build"},
		{name: "env helper build", script: `env CGO_ENABLED=0 go build -o "$RUNNER_TEMP/helper" ./cmd/helper`},
		{name: "env helper chdir", script: "env -C ./cmd/helper go build"},
		{name: "step helper chdir", script: "go build", workingDirectory: "./cmd/helper"},
		{name: "absolute external helper chdir", script: "go build", workingDirectory: "/tmp/helper"},
		{name: "runner temp helper chdir", script: "go build", workingDirectory: `${{ runner.temp }}/helper`},
		{name: "go helper chdir", script: "go -C ./cmd/helper build"},
		{name: "shell helper chdir", script: "cd ./cmd/helper && go build"},
		{name: "quoted command data", script: `printf '%s\\n' 'go build -o verity .'`},
		{name: "non-Verity output", script: `go build -o "$RUNNER_TEMP/verity-check" ./cmd/helper`},
		{name: "helper install", script: "go install ./cmd/helper"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a shell command in a repository workflow step.
			location := runLocation{
				file:             "integer-build-image-reusable.yaml",
				job:              "build",
				workingDirectory: test.workingDirectory,
				isWorkflow:       true,
			}

			// When local compilation policy classifies the command.
			violations := localVerityBuildViolations(location, test.script)

			// Then Verity compilation variants fail closed and helper builds remain allowed.
			if test.wantViolation {
				require.Len(t, violations, 1)
				assert.Equal(t, RuleBuildOnce, violations[0].Rule)
				return
			}
			assert.Empty(t, violations)
		})
	}
}

func copyLocalBuildPolicyRepository(t *testing.T) string {
	t.Helper()

	root := copyBuildOnceRepository(t)
	data := readBuildOnceFixture(t, ".github", "actions", "setup-binaries", "action.yml")
	path := filepath.Join(root, ".github", "actions", "setup-binaries", "action.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return root
}

func readLocalBuildMutations(t *testing.T) []localBuildMutation {
	t.Helper()

	data := readBuildOnceFixture(t, "local-build-mutations.yaml")
	var mutations []localBuildMutation
	require.NoError(t, yaml.Unmarshal(data, &mutations))
	require.NotEmpty(t, mutations)
	return mutations
}
