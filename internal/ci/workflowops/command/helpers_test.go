package command

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

var errTestAggregation = errors.New("aggregation failed")

func TestOpenAppend_empty_path_does_not_create_typed_nil_closer(t *testing.T) {
	// Given: an aggregation command without a GitHub step-summary path.
	baseErr := errTestAggregation
	closer, _, err := openAppend("")
	require.NoError(t, err)

	// When: command cleanup joins any close error.
	err = closeWithError(baseErr, closer, "GitHub step summary")

	// Then: no typed-nil close error obscures the aggregation failure.
	require.Equal(t, baseErr, err)
}
