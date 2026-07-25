package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGatesEquivalent_accepts_only_same_boolean_contract(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
		want     bool
	}{
		{
			name:     "whitespace and conjunction order are irrelevant",
			actual:   "${{ b == 'success' && a == github.sha }}",
			expected: "a==github.sha&&b=='success'",
			want:     true,
		},
		{
			name:     "or true is not equivalent",
			actual:   "${{ a == github.sha || true }}",
			expected: "a == github.sha",
		},
		{
			name:     "false inversion is not equivalent",
			actual:   "${{ !((a == github.sha) && false) }}",
			expected: "a == github.sha",
		},
		{
			name:     "always is not success result",
			actual:   "${{ always() }}",
			expected: "needs.build.result == 'success'",
		},
		{
			name:     "not cancelled is not success result",
			actual:   "${{ !cancelled() }}",
			expected: "needs.build.result == 'success'",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, gatesEquivalent(test.actual, test.expected))
		})
	}
}
