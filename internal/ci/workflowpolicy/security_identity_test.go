package workflowpolicy

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegerWritePermissionDelegations_acceptExactRepositoryIdentities(t *testing.T) {
	// Given every legitimate direct, reusable, and shard delegation in the Integer graph.
	workflows := repositoryIntegerWorkflowFiles(t)
	expectedDelegations := expectedPermissionDelegations()

	// When the exact permission identity table is compared with the repository jobs.
	require.ElementsMatch(t, expectedDelegations, integerPermissionDelegations)
	for _, delegation := range expectedDelegations {
		file, exists := findWorkflow(workflows, delegation.identity.workflow)
		require.True(t, exists, delegation.identity.workflow)
		job, exists := file.Workflow.Jobs[delegation.identity.job]
		require.True(t, exists, "%s:%s", delegation.identity.workflow, delegation.identity.job)
		require.Equal(t, delegation.uses, job.Uses)

		// Then every declared delegated scope is accepted only for that exact identity.
		for _, scope := range delegation.scopes {
			assert.True(t, integerDelegatesWritePermission(delegation.identity, &job, scope),
				"%s:%s %s", delegation.identity.workflow, delegation.identity.job, scope)
		}
	}
}

func TestValidateDirectory_rejectsIntegerWritePermissionIdentityLookalikes(t *testing.T) {
	for _, delegation := range expectedPermissionDelegations() {
		identity := delegation.identity
		t.Run(identity.workflow+"/"+identity.job, func(t *testing.T) {
			for name, filename := range workflowFilenameLookalikes(identity.workflow) {
				t.Run("caller_"+name, func(t *testing.T) {
					// Given a lookalike workflow with the exact privileged job name and child workflow.
					root := copyWorkflowFixture(t, "valid")
					require.NoError(t, os.WriteFile(
						filepath.Join(root, filename),
						renderPermissionIdentityDecoy(identity.job, delegation.uses, delegation.scopes),
						0o600,
					))

					// When the complete workflow policy evaluates the caller lookalike.
					requireUnusedWriteViolations(t, unusedWriteExpectation{
						root: root, workflow: filename, job: identity.job, scopes: delegation.scopes,
					})
				})
			}

			for name, uses := range workflowUsesLookalikes(delegation.uses) {
				t.Run("uses_"+name, func(t *testing.T) {
					// Given the exact workflow and job redirected to a path lookalike.
					root := copyWorkflowFixture(t, "valid")
					workflowPath := filepath.Join(root, identity.workflow)
					data, err := os.ReadFile(workflowPath)
					require.NoError(t, err)
					old := "    uses: " + delegation.uses
					replacement := "    uses: " + uses
					require.Contains(t, string(data), old, "stale identity fixture")
					require.NoError(t, os.WriteFile(
						workflowPath,
						[]byte(strings.Replace(string(data), old, replacement, 1)),
						0o600,
					))

					// When the complete workflow policy evaluates the uses lookalike.
					requireUnusedWriteViolations(t, unusedWriteExpectation{
						root: root, workflow: identity.workflow, job: identity.job, scopes: delegation.scopes,
					})
				})
			}
		})
	}
}

func expectedPermissionDelegations() []integerPermissionDelegation {
	rows := []struct {
		workflow string
		job      string
		uses     string
		packages bool
	}{
		{workflow: "integer-orchestrator.yaml", job: "build-verity", uses: protectedBuildVerityWorkflowReference},
		{workflow: "integer-orchestrator.yaml", job: "orchestrate", uses: integerOrchestratorReference, packages: true},
		{workflow: "integer-build-image.yaml", job: "build-verity", uses: protectedBuildVerityWorkflowReference},
		{workflow: "integer-build-image.yaml", job: "build", uses: integerImageWorkflowReference, packages: true},
		{workflow: "integer-orchestrator-reusable.yaml", job: "build-shards", uses: integerShardWorkflowReference, packages: true},
		{workflow: "integer-build-shard.yaml", job: "build", uses: integerImageWorkflowReference, packages: true},
	}
	delegations := make([]integerPermissionDelegation, 0, len(rows))
	for _, row := range rows {
		scopes := []permissionScope{idTokenScope, attestationsScope}
		if row.packages {
			scopes = []permissionScope{packagesScope, idTokenScope, attestationsScope}
		}
		delegations = append(delegations, integerPermissionDelegation{
			identity: workflowJobIdentity{workflow: row.workflow, job: row.job},
			uses:     row.uses,
			scopes:   scopes,
		})
	}
	return delegations
}

func workflowFilenameLookalikes(filename string) map[string]string {
	extension := path.Ext(filename)
	stem := strings.TrimSuffix(filename, extension)
	return map[string]string{
		"prefix": "copy-" + filename,
		"suffix": stem + "-copy" + extension,
		"case":   strings.ToUpper(filename[:1]) + filename[1:],
	}
}

func workflowUsesLookalikes(uses string) map[string]string {
	filename := path.Base(uses)
	prefix := strings.TrimSuffix(uses, filename)
	extension := path.Ext(filename)
	stem := strings.TrimSuffix(filename, extension)
	return map[string]string{
		"prefix":    prefix + "copy-" + filename,
		"suffix":    prefix + stem + "-copy" + extension,
		"case":      prefix + strings.ToUpper(filename[:1]) + filename[1:],
		"nested":    prefix + "nested/" + filename,
		"traversal": prefix + "../workflows/" + filename,
	}
}

func renderPermissionIdentityDecoy(jobName, uses string, scopes []permissionScope) []byte {
	var document strings.Builder
	fmt.Fprintf(&document, `name: Permission identity decoy

on: workflow_dispatch

permissions:
  contents: read

jobs:
  %s:
    uses: %s
    permissions:
      contents: read
`, jobName, uses)
	for _, scope := range scopes {
		fmt.Fprintf(&document, "      %s: write\n", scope)
	}
	return []byte(document.String())
}

type unusedWriteExpectation struct {
	root     string
	workflow string
	job      string
	scopes   []permissionScope
}

func requireUnusedWriteViolations(t *testing.T, expectation unusedWriteExpectation) {
	t.Helper()

	_, err := ValidateDirectory(expectation.root)
	require.Error(t, err)
	var policyError *PolicyError
	require.True(t, errors.As(err, &policyError))
	for _, scope := range expectation.scopes {
		found := false
		for _, violation := range policyError.Violations {
			if violation.Rule == RuleLeastPrivilege &&
				violation.Workflow == expectation.workflow &&
				violation.Job == expectation.job &&
				violation.Detail == fmt.Sprintf("%s: write is not used by this job", scope) {
				found = true
				break
			}
		}
		assert.True(t, found, "lookalike inherited %s: write:\n%s", scope, err)
	}
}
