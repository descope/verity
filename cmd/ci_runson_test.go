package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/runson"
)

func TestCIRunsOnVerify_rejectsInvalidRequirements(t *testing.T) {
	// Given the RunsOn verifier command with an invalid account boundary.
	command := &cli.Command{Name: "ci", Commands: []*cli.Command{newCIRunsOnCommand()}}
	arguments := []string{
		"ci", "runs-on", "verify",
		"--expected-account", "invalid",
		"--expected-region", "us-east-1",
		"--expected-arch", "amd64",
		"--minimum-cpu", "4",
		"--minimum-memory-gib", "7",
		"--minimum-disk-gib", "30",
	}

	// When the command parses the verification requirements.
	err := command.Run(context.Background(), arguments)

	// Then the boundary rejects the request before probing the host.
	require.Error(t, err)
	assert.ErrorIs(t, err, runson.ErrInvalidRequirements)
}
