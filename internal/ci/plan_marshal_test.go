package ci

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshal_normalizes_empty_plan_matrices_deterministically(t *testing.T) {
	// Given
	plan := Plan{Kind: "integer-pr"}

	// When
	first, err := Marshal(plan)
	require.NoError(t, err)
	second, err := Marshal(plan)
	require.NoError(t, err)
	var decoded Plan
	err = json.Unmarshal(first, &decoded)

	// Then
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NotNil(t, decoded.Matrix.Include)
	require.NotNil(t, decoded.SmokeMatrix)
	require.NotNil(t, decoded.SmokeMatrix.Include)
	require.Nil(t, plan.SmokeMatrix)
}
