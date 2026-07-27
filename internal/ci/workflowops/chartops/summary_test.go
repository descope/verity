package chartops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSummary_renders_tracked_skip_for_successful_standard_shard(t *testing.T) {
	// Given a successful shard with the tracked skip sentinel emitted by the harness.
	sentinel := filepath.Join(t.TempDir(), "_skip-falco.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte(
		"chart=falco\nreason=requires privileged eBPF\ntracking_issue=https://example.invalid/42\n"+
			"exit_criteria=runner supports eBPF\nadded=2026-01-01\nadded_by=test\n",
	), 0o600))

	// When the standard shard summary is built.
	summary, err := BuildSummary(SummaryInput{
		Chart: "falco", Outcome: "success", Profile: ProfileStandard, SkipFile: sentinel,
	})

	// Then the result is reported as an explicit tracked skip, not a success.
	require.NoError(t, err)
	assert.Contains(t, summary, "## ⚠️ falco: skipped (SKIPS.yaml)")
	assert.Contains(t, summary, "requires privileged eBPF")
	assert.Contains(t, summary, "https://example.invalid/42")
	assert.NotContains(t, summary, "## ✅ falco: success")
}

func TestBuildSummary_rejects_incomplete_skip_sentinel(t *testing.T) {
	// Given a successful shard whose sentinel omits required tracking fields.
	sentinel := filepath.Join(t.TempDir(), "_skip-falco.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("reason=missing tracking\n"), 0o600))

	// When the standard shard summary is built.
	_, err := BuildSummary(SummaryInput{
		Chart: "falco", Outcome: "success", Profile: ProfileStandard, SkipFile: sentinel,
	})

	// Then malformed skip metadata fails closed.
	require.ErrorIs(t, err, ErrInvalidSkipSentinel)
}

func TestBuildSummary_renders_privileged_failure(t *testing.T) {
	// Given a failed privileged shard.
	input := SummaryInput{Chart: "cilium", Outcome: "failure", Profile: ProfilePrivileged}

	// When its summary is built.
	summary, err := BuildSummary(input)

	// Then the privileged diagnostics artifact is named in the failure guidance.
	require.NoError(t, err)
	assert.Contains(t, summary, "## ❌ cilium privileged: failure")
	assert.Contains(t, summary, "diagnostics-cilium-privileged")
}
