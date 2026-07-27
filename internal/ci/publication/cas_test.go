package publication

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareAndSwap_rejects_stale_state_and_replay(t *testing.T) {
	// Given three valid publication digests.
	current := Digest("sha256:" + strings.Repeat("1", 64))
	stale := Digest("sha256:" + strings.Repeat("2", 64))
	next := Digest("sha256:" + strings.Repeat("3", 64))

	// When expected state matches, then the transition is accepted.
	require.NoError(t, CompareAndSwap(current, current, next))

	// When expected state is stale, then CAS rejects it.
	require.ErrorIs(t, CompareAndSwap(current, stale, next), ErrCASMismatch)

	// When next state repeats current bytes, then replay is rejected.
	require.ErrorIs(t, CompareAndSwap(current, current, current), ErrReplay)
}
