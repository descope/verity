package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeWorkflow_rejects_duplicate_mapping_keys_at_every_policy_boundary(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "workflow", yaml: "name: one\nname: two\non: workflow_dispatch\njobs: {}\n"},
		{name: "trigger", yaml: "on:\n  workflow_dispatch:\n  workflow_dispatch:\njobs: {}\n"},
		{name: "trigger configuration", yaml: "on:\n  push:\n    paths: [one]\n    paths: [two]\njobs: {}\n"},
		{name: "permissions", yaml: "on: workflow_dispatch\npermissions:\n  contents: read\n  contents: write\njobs: {}\n"},
		{name: "jobs", yaml: "on: workflow_dispatch\njobs:\n  build: {permissions: {contents: read}}\n  build: {permissions: {contents: read}}\n"},
		{name: "job", yaml: "on: workflow_dispatch\njobs:\n  build:\n    permissions: {contents: read}\n    permissions: {contents: read}\n"},
		{name: "steps", yaml: "on: workflow_dispatch\njobs:\n  build:\n    steps: []\n    steps: []\n"},
		{name: "step", yaml: "on: workflow_dispatch\njobs:\n  build:\n    steps:\n      - uses: actions/checkout@1111111111111111111111111111111111111111\n        uses: actions/checkout@2222222222222222222222222222222222222222\n"},
		{name: "job with", yaml: "on: workflow_dispatch\njobs:\n  build:\n    with: {name: one, name: two}\n"},
		{name: "step with", yaml: "on: workflow_dispatch\njobs:\n  build:\n    steps:\n      - uses: example/action@1111111111111111111111111111111111111111\n        with: {name: one, name: two}\n"},
		{name: "step env", yaml: "on: workflow_dispatch\njobs:\n  build:\n    steps:\n      - run: true\n        env: {TOKEN: one, TOKEN: two}\n"},
		{name: "workflow call inputs", yaml: "on:\n  workflow_call:\n    inputs:\n      batch_id: {required: true, type: string}\n      batch_id: {required: true, type: string}\njobs: {}\n"},
		{name: "workflow call input", yaml: "on:\n  workflow_call:\n    inputs:\n      batch_id: {required: true, required: false, type: string}\njobs: {}\n"},
		{name: "outputs", yaml: "on: workflow_dispatch\njobs:\n  build:\n    outputs: {artifact: one, artifact: two}\n"},
		{name: "matrix", yaml: "on: workflow_dispatch\njobs:\n  build:\n    strategy:\n      matrix: {shard: [one], shard: [two]}\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When ambiguous YAML is decoded at the workflow boundary.
			_, err := decodeWorkflow([]byte(test.yaml))

			// Then duplicate keys fail closed instead of using a last value.
			assert.Error(t, err)
		})
	}
}

func TestDecodeWorkflow_rejects_unknown_fields_and_case_variants(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "workflow", yaml: "name: test\nunknown-policy-field: true\non: workflow_dispatch\njobs: {}\n"},
		{name: "workflow case variant", yaml: "Name: test\non: workflow_dispatch\njobs: {}\n"},
		{name: "job", yaml: "on: workflow_dispatch\njobs:\n  build:\n    permissions: {contents: read}\n    unknown-job-field: true\n"},
		{name: "job case variant", yaml: "on: workflow_dispatch\njobs:\n  build:\n    Runs-On: ubuntu-24.04\n"},
		{name: "step", yaml: "on: workflow_dispatch\njobs:\n  build:\n    steps:\n      - run: true\n        unknown-step-field: true\n"},
		{name: "step case variant", yaml: "on: workflow_dispatch\njobs:\n  build:\n    steps:\n      - Run: true\n"},
		{name: "trigger", yaml: "on:\n  push:\n    paths: [one]\n    unknown-trigger-field: true\njobs: {}\n"},
		{name: "unsupported trigger filter", yaml: "on:\n  deployment:\n    types: [created]\njobs: {}\n"},
		{name: "trigger case variant", yaml: "on:\n  push:\n    Paths: [one]\njobs: {}\n"},
		{name: "permission", yaml: "on: workflow_dispatch\npermissions:\n  imaginary-scope: read\njobs: {}\n"},
		{name: "permission case variant", yaml: "on: workflow_dispatch\npermissions:\n  Contents: read\njobs: {}\n"},
		{name: "identity input", yaml: "on:\n  workflow_call:\n    inputs:\n      batch_id:\n        required: true\n        type: string\n        unknown-contract-field: true\njobs: {}\n"},
		{name: "identity input case variant", yaml: "on:\n  workflow_call:\n    inputs:\n      batch_id:\n        Required: true\n        type: string\njobs: {}\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When a non-schema field is decoded at a typed policy boundary.
			_, err := decodeWorkflow([]byte(test.yaml))

			// Then unknown and noncanonical keys fail closed.
			assert.Error(t, err)
		})
	}
}

func TestDecodeWorkflow_rejects_YAML_graph_features(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "anchor", yaml: "name: &workflow_name test\non: workflow_dispatch\njobs: {}\n"},
		{name: "key anchor", yaml: "&workflow_name name: test\non: workflow_dispatch\njobs: {}\n"},
		{name: "alias", yaml: "name: &workflow_name test\nrun-name: *workflow_name\non: workflow_dispatch\njobs: {}\n"},
		{name: "merge key", yaml: "on: workflow_dispatch\npermissions:\n  <<: {contents: read}\njobs: {}\n"},
		{name: "custom tag", yaml: "name: !policy test\non: workflow_dispatch\njobs: {}\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When YAML graph indirection reaches the policy parser.
			_, err := decodeWorkflow([]byte(test.yaml))

			// Then aliases, anchors, and merge semantics are rejected.
			assert.Error(t, err)
		})
	}
}

func TestDecodeWorkflow_preserves_deliberate_extension_maps_and_expressions(t *testing.T) {
	// Given arbitrary extension keys in env, with, outputs, and matrix dimensions.
	input := `name: extensions
run-name: ${{ github.workflow }}-${{ github.run_id }}
on: workflow_dispatch
permissions:
  contents: read
env:
  CUSTOM_ENV: ${{ github.sha }}
jobs:
  build:
    runs-on: ubuntu-24.04
    permissions:
      contents: read
    outputs:
      custom-output: ${{ steps.publish.outputs.digest }}
    strategy:
      matrix:
        custom-dimension: [one, two]
    steps:
      - id: publish
        uses: example/action@1111111111111111111111111111111111111111
        with:
          custom-input: ${{ matrix.custom-dimension }}
        env:
          STEP_ENV: ${{ github.ref }}
`

	// When the workflow crosses the strict typed boundary.
	_, err := decodeWorkflow([]byte(input))

	// Then deliberate extension maps and expression scalars remain valid.
	assert.NoError(t, err)
}
