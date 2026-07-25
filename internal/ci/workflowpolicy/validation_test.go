package workflowpolicy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workflowMutation struct {
	workflow    string
	old         string
	replacement string
}

func TestValidateDirectory_accepts_compliant_workflows(t *testing.T) {
	// Given a fixture containing the complete typed workflow contract.
	root := filepath.Join("testdata", "valid")

	// When the workflow policy is evaluated.
	report, err := ValidateDirectory(root)

	// Then every workflow passes and the report records the parsed set.
	require.NoError(t, err)
	assert.Equal(t, 9, report.WorkflowCount)
}

func TestValidateDirectory_rejects_policy_mutations(t *testing.T) {
	tests := []struct {
		name     string
		mutation workflowMutation
		rule     Rule
	}{
		{
			name: "APK Pages permission",
			mutation: workflowMutation{
				workflow:    "apk-repository.yaml",
				old:         "permissions:\n  contents: read",
				replacement: "permissions:\n  contents: read\n  pages: write",
			},
			rule: RuleAPKPagesPermission,
		},
		{
			name: "missing Integer recipe trigger",
			mutation: workflowMutation{
				workflow: "integer-orchestrator.yaml",
				old:      "      - \"packages/pipelines/**\"\n",
			},
			rule: RuleIntegerTriggers,
		},
		{
			name: "stale APK artifact identity",
			mutation: workflowMutation{
				workflow:    "integer-build-image-reusable.yaml",
				old:         integerComponentArtifactName,
				replacement: "apk-repository-latest",
			},
			rule: RuleProducerIdentity,
		},
		{
			name: "publish caller excess permission",
			mutation: workflowMutation{
				workflow:    "publish.yaml",
				old:         "      pages: write\n    uses: ./.github/workflows/build-site.yaml",
				replacement: "      pages: write\n      issues: write\n    uses: ./.github/workflows/build-site.yaml",
			},
			rule: RuleLeastPrivilege,
		},
		{
			name: "floating action ref",
			mutation: workflowMutation{
				workflow:    "apk-repository.yaml",
				old:         "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0",
				replacement: "actions/checkout@main",
			},
			rule: RulePinnedReference,
		},
		{
			name: "floating inline image",
			mutation: workflowMutation{
				workflow:    "apk-repository.yaml",
				old:         "./verity ci apk-repository validate site/dist/apk",
				replacement: "docker run alpine:latest true",
			},
			rule: RulePinnedReference,
		},
		{
			name: "workflow-level write permission",
			mutation: workflowMutation{
				workflow:    "build-site.yaml",
				old:         "permissions: {}",
				replacement: "permissions:\n  pages: write",
			},
			rule: RuleLeastPrivilege,
		},
		{
			name: "unused job write permission",
			mutation: workflowMutation{
				workflow:    "apk-repository.yaml",
				old:         "    permissions:\n      contents: read\n    steps:",
				replacement: "    permissions:\n      contents: read\n      issues: write\n    steps:",
			},
			rule: RuleLeastPrivilege,
		},
		{
			name: "PR package write",
			mutation: workflowMutation{
				workflow:    "pr-test.yaml",
				old:         "      packages: read",
				replacement: "      packages: write",
			},
			rule: RulePRPackagesWrite,
		},
		{
			name: "workflow-owned Python",
			mutation: workflowMutation{
				workflow:    "apk-repository.yaml",
				old:         "./verity ci apk-repository validate site/dist/apk",
				replacement: "python3 .github/scripts/validate-workflow.py",
			},
			rule: RuleGoOwnedLogic,
		},
		{
			name: "weakened local Trivy severities",
			mutation: workflowMutation{
				workflow: "integer-build-image-reusable.yaml",
				old: `--arch amd64 --fail-on-severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL --output gate-amd64.tar
          ./verity integer build --image "$INPUT_IMAGE" --version "$INPUT_VERSION" --type "$INPUT_TYPE" --arch arm64 --fail-on-severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL --output gate-arm64.tar`,
				replacement: `--arch amd64 --fail-on-severity CRITICAL --output gate-amd64.tar
          ./verity integer build --image "$INPUT_IMAGE" --version "$INPUT_VERSION" --type "$INPUT_TYPE" --arch arm64 --fail-on-severity CRITICAL --output gate-arm64.tar`,
			},
			rule: RuleZeroCVEOrdering,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one isolated mutation of the compliant fixture.
			root := copyWorkflowFixture(t, "valid")
			replaceWorkflowText(t, root, test.mutation)

			// When the workflow policy is evaluated.
			_, err := ValidateDirectory(root)

			// Then the named fail-closed rule rejects the mutation.
			require.Error(t, err)
			require.ErrorIs(t, err, ErrPolicyViolation)
			var policyError *PolicyError
			require.True(t, errors.As(err, &policyError))
			assert.Contains(t, violationRules(policyError.Violations), test.rule)
		})
	}
}

func TestValidateDirectory_rejects_malformed_YAML(t *testing.T) {
	// Given a workflow directory containing syntactically invalid YAML.
	root := copyWorkflowFixture(t, "valid")
	require.NoError(t, os.WriteFile(filepath.Join(root, "broken.yaml"), []byte("jobs: ["), 0o600))

	// When the workflow policy is evaluated.
	_, err := ValidateDirectory(root)

	// Then parsing fails closed before policy success can be reported.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidWorkflow)
}

func TestValidateZeroCVEOrdering_rejects_publication_before_gate(t *testing.T) {
	// Given: parsed publication is moved ahead of the fail-closed local gate.
	workflows := repositoryIntegerWorkflowFiles(t)
	require.Equal(t, "integer-build-image-reusable.yaml", workflows[2].Name)
	build := workflows[2].Workflow.Jobs["build"]
	gateIndex, publishIndex := -1, -1
	for index, step := range build.Steps {
		if strings.Contains(step.Run, "--fail-on-severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL") {
			gateIndex = index
		}
		if strings.Contains(step.Run, "./verity ci integer-image publish") {
			publishIndex = index
		}
	}
	require.GreaterOrEqual(t, gateIndex, 0)
	require.GreaterOrEqual(t, publishIndex, 0)
	build.Steps[gateIndex], build.Steps[publishIndex] = build.Steps[publishIndex], build.Steps[gateIndex]
	workflows[2].Workflow.Jobs["build"] = build

	// When/Then: parsed zero-CVE policy rejects the reordered topology.
	violations := validateZeroCVEOrdering(workflows)
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleZeroCVEOrdering)
}

func TestValidateZeroCVEOrdering_rejects_GoOwnedPublicationBypass(t *testing.T) {
	// Given: the parsed typed publisher is configured to ignore failure.
	workflows := repositoryIntegerWorkflowFiles(t)
	build := workflows[2].Workflow.Jobs["build"]
	publishIndex := -1
	for index, step := range build.Steps {
		if strings.Contains(step.Run, "./verity ci integer-image publish") {
			publishIndex = index
			break
		}
	}
	require.GreaterOrEqual(t, publishIndex, 0)
	build.Steps[publishIndex].ContinueOnError = scalarValue{set: true, value: "true"}
	workflows[2].Workflow.Jobs["build"] = build

	// When/Then: staged Trivy remains a mandatory fail-closed boundary.
	violations := validateZeroCVEOrdering(workflows)
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleZeroCVEOrdering)
}

func replaceWorkflowText(t *testing.T, root string, mutation workflowMutation) {
	t.Helper()

	path := filepath.Join(root, mutation.workflow)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), mutation.old, "stale fixture mutation for %s", mutation.workflow)
	updated := []byte(strings.Replace(string(data), mutation.old, mutation.replacement, 1))
	require.NoError(t, os.WriteFile(path, updated, 0o600))
}

func violationRules(violations []Violation) []Rule {
	rules := make([]Rule, 0, len(violations))
	for _, violation := range violations {
		rules = append(rules, violation.Rule)
	}
	return rules
}
