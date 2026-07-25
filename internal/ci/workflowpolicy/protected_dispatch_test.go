package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateProtectedDispatchRefs_accepts_only_exact_github_SHA_expressions(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{name: "compact property", ref: "${{github.sha}}"},
		{name: "spaced property", ref: "${{ github.sha }}"},
		{name: "parenthesized property", ref: "${{ (github.sha) }}"},
		{name: "single quoted index", ref: "${{ github['sha'] }}"},
		{name: "double quoted index", ref: "${{ github[\"sha\"] }}"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a manually dispatched checkout bound exactly to github.sha.
			workflows := protectedDispatchWorkflows(test.ref)

			// When protected checkout refs are evaluated.
			violations := validateProtectedDispatchRefs(workflows)

			// Then equivalent exact expressions remain allowed.
			assert.Empty(t, violations)
		})
	}
}

func TestValidateProtectedDispatchRefs_rejects_inexact_github_SHA_expressions(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{name: "fallback", ref: "${{ inputs.ref || github.sha }}"},
		{name: "interpolation", ref: "refs/heads/${{ github.sha }}"},
		{name: "different SHA", ref: "${{ github.event.pull_request.head.sha }}"},
		{name: "format call", ref: "${{ format('{0}', github.sha) }}"},
		{name: "whitespace inside indexed key", ref: "${{ github['s h a'] }}"},
		{name: "unbalanced parenthesis", ref: "${{ (github.sha }}"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a checkout ref that merely contains or derives from github.sha.
			workflows := protectedDispatchWorkflows(test.ref)

			// When protected checkout refs are evaluated.
			violations := validateProtectedDispatchRefs(workflows)

			// Then anything not provably identical remains fail-closed.
			assert.Contains(t, violationRules(violations), RuleProtectedDispatch)
		})
	}
}

func protectedDispatchWorkflows(ref string) []workflowFile {
	return []workflowFile{{Name: "dispatch.yaml", Workflow: workflow{
		On: triggers{WorkflowDispatch: true},
		Jobs: map[string]workflowJob{"checkout": {Steps: []workflowStep{{
			Uses: "actions/checkout@1111111111111111111111111111111111111111",
			With: scalarMap{"ref": ref},
		}}}},
	}}}
}
