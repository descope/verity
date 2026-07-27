package repositoryops_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

func TestAppendWorkflowValues_rejectsMultilineInjectionWithoutWriting(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "github-output")

	// When
	err := ops.AppendWorkflowValues(path, []ops.WorkflowValue{{Name: "source", Value: "safe\nowned=true"}})

	// Then
	require.ErrorIs(t, err, ops.ErrInvalidWorkflowOutput)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}
