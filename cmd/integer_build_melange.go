package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

var integerMelangeBuildScriptPath = filepath.Join(".github", "scripts", "melange-build.sh")

const (
	integerMelangeRepoDir = "packages/repo"
	integerMelangeKeyPath = "melange-work/melange.rsa.pub"
)

func integerPrepareMelangeBuild(ctx context.Context, melange *intconfig.MelangeSpec) (repos, keyrings []string, err error) {
	if melange == nil {
		return nil, nil, nil
	}

	bespokeJSON, err := json.Marshal([]string(melange.Bespoke))
	if err != nil {
		return nil, nil, fmt.Errorf("marshal bespoke list: %w", err)
	}

	cmd := exec.CommandContext(ctx, "bash", integerMelangeBuildScriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(
		os.Environ(),
		"BESPOKE_JSON="+string(bespokeJSON),
		"UPSTREAM="+melange.Upstream,
		"ENV_FILE="+melange.EnvFile,
		"BUILD_OPTION="+melange.BuildOption,
	)
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("run %s: %w", integerMelangeBuildScriptPath, err)
	}
	if _, err := os.Stat(integerMelangeRepoDir); err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", integerMelangeRepoDir, err)
	}
	if _, err := os.Stat(integerMelangeKeyPath); err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", integerMelangeKeyPath, err)
	}

	return []string{integerMelangeRepoDir}, []string{integerMelangeKeyPath}, nil
}
