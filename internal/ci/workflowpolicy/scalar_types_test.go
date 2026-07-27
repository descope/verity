package workflowpolicy

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeWorkflow_rejects_non_string_scalars_recursively_at_string_boundaries(t *testing.T) {
	boundaries := []struct {
		name     string
		template string
	}{
		{name: "workflow name", template: "name: %s\non: workflow_dispatch\njobs: {}\n"},
		{name: "run name", template: "run-name: %s\non: workflow_dispatch\njobs: {}\n"},
		{name: "concurrency expression", template: "on: workflow_dispatch\nconcurrency: %s\njobs: {}\n"},
		{name: "job name", template: "on: workflow_dispatch\njobs:\n  build:\n    name: %s\n"},
		{name: "job condition", template: "on: workflow_dispatch\njobs:\n  build:\n    if: %s\n"},
		{name: "job output", template: "on: workflow_dispatch\njobs:\n  build:\n    outputs: {digest: %s}\n"},
		{name: "step name", template: "on: workflow_dispatch\njobs:\n  build:\n    steps: [{name: %s, run: echo}]\n"},
		{name: "step action", template: "on: workflow_dispatch\njobs:\n  build:\n    steps: [{uses: %s}]\n"},
		{name: "step command", template: "on: workflow_dispatch\njobs:\n  build:\n    steps: [{run: %s}]\n"},
		{name: "step condition", template: "on: workflow_dispatch\njobs:\n  build:\n    steps: [{if: %s, run: echo}]\n"},
		{name: "step input", template: "on: workflow_dispatch\njobs:\n  build:\n    steps: [{uses: example/action@1111111111111111111111111111111111111111, with: {name: %s}}]\n"},
		{name: "step environment", template: "on: workflow_dispatch\njobs:\n  build:\n    steps: [{run: echo, env: {VALUE: %s}}]\n"},
		{name: "default shell", template: "on: workflow_dispatch\ndefaults: {run: {shell: %s}}\njobs: {}\n"},
		{name: "matrix expression", template: "on: workflow_dispatch\njobs:\n  build:\n    strategy: {matrix: %s}\n"},
		{name: "input description", template: "on:\n  workflow_call:\n    inputs:\n      batch_id: {description: %s, required: true, type: string}\njobs: {}\n"},
		{name: "input type", template: "on:\n  workflow_call:\n    inputs:\n      batch_id: {required: true, type: %s}\njobs: {}\n"},
		{name: "string input default", template: "on:\n  workflow_call:\n    inputs:\n      batch_id: {required: true, type: string, default: %s}\njobs: {}\n"},
		{name: "reusable output value", template: "on:\n  workflow_call:\n    outputs:\n      artifact: {value: %s}\njobs: {}\n"},
	}
	mutations := []struct {
		name  string
		value string
	}{
		{name: "null", value: "null"},
		{name: "boolean", value: "true"},
		{name: "integer", value: "42"},
		{name: "float", value: "1.5"},
		{name: "explicit tag", value: "!!str tagged"},
		{name: "custom tag", value: "!policy tagged"},
	}

	for _, boundary := range boundaries {
		for _, mutation := range mutations {
			t.Run(boundary.name+"/"+mutation.name, func(t *testing.T) {
				// When a noncanonical scalar is recursively injected at a string boundary.
				_, err := decodeWorkflow(fmt.Appendf(nil, boundary.template, mutation.value))

				// Then the typed boundary rejects it rather than coercing or discarding it.
				assert.Error(t, err)
			})
		}
	}
}

func TestDecodeWorkflow_rejects_mistyped_boolean_number_and_nested_matrix_scalars(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "null concurrency flag", yaml: "on: workflow_dispatch\nconcurrency: {group: publish, cancel-in-progress: null}\njobs: {}\n"},
		{name: "quoted concurrency flag", yaml: "on: workflow_dispatch\nconcurrency: {group: publish, cancel-in-progress: \"false\"}\njobs: {}\n"},
		{name: "boolean timeout", yaml: "on: workflow_dispatch\njobs: {build: {timeout-minutes: false}}\n"},
		{name: "quoted timeout", yaml: "on: workflow_dispatch\njobs: {build: {timeout-minutes: \"10\"}}\n"},
		{name: "quoted required flag", yaml: "on:\n  workflow_call:\n    inputs:\n      batch: {required: \"true\", type: string}\njobs: {}\n"},
		{name: "string boolean default", yaml: "on:\n  workflow_call:\n    inputs:\n      enabled: {required: true, type: boolean, default: \"false\"}\njobs: {}\n"},
		{name: "string number default", yaml: "on:\n  workflow_call:\n    inputs:\n      count: {required: true, type: number, default: \"2\"}\njobs: {}\n"},
		{name: "nested null matrix value", yaml: "on: workflow_dispatch\njobs:\n  build:\n    strategy:\n      matrix:\n        include: [{runner: null}]\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When a typed scalar position receives a coercible or null value.
			_, err := decodeWorkflow([]byte(test.yaml))

			// Then the boundary rejects it instead of relying on YAML coercion.
			assert.Error(t, err)
		})
	}
}

func TestDecodeWorkflow_accepts_deliberately_typed_boolean_number_and_matrix_values(t *testing.T) {
	// Given schema fields whose GitHub contract deliberately permits typed values.
	input := `on:
  workflow_dispatch:
    inputs:
      enabled: {required: true, type: boolean, default: false}
      count: {required: true, type: number, default: 2}
permissions:
  contents: read
concurrency:
  group: publication
  cancel-in-progress: false
jobs:
  build:
    timeout-minutes: 10
    continue-on-error: false
    strategy:
      fail-fast: false
      max-parallel: 2
      matrix:
        include:
          - runner: ubuntu-24.04
            enabled: true
            shard: 1
    runs-on: ubuntu-24.04
    permissions:
      contents: read
    steps:
      - run: echo valid
  call:
    uses: ./.github/workflows/reusable.yaml
    with:
      enabled: false
      count: 2
      name: publication
`

	// When the typed scalar forms cross the workflow boundary.
	_, err := decodeWorkflow([]byte(input))

	// Then deliberate booleans, numbers, and heterogeneous matrix values remain valid.
	assert.NoError(t, err)
}

func TestDecodeWorkflow_rejects_nonScalarReusableInputs(t *testing.T) {
	tests := []string{
		"null",
		"[one, two]",
		"{nested: value}",
	}
	for _, value := range tests {
		// Given: a reusable workflow input with a non-supported value shape.
		input := fmt.Sprintf("on: workflow_dispatch\njobs:\n  call:\n    uses: ./.github/workflows/reusable.yaml\n    with: {value: %s}\n", value)

		// When: the strict schema validates the input mapping.
		_, err := decodeWorkflow([]byte(input))

		// Then: only GitHub-supported scalar input values are accepted.
		assert.Error(t, err)
	}
}
