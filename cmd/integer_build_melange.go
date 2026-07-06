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

func integerPrepareMelangeBuild(ctx context.Context, melange *intconfig.MelangeSpec, arch string) (repos, keyrings []string, err error) {
	if melange == nil {
		return nil, nil, nil
	}

	melangeArch := integerMelangeArch(arch)
	if integerMelangeArtifactsExist(melangeArch) {
		return []string{integerMelangeRepoDir}, []string{integerMelangeKeyPath}, nil
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
		"BUILD_ARCH="+melangeArch,
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

func integerMelangeArtifactsExist(arch string) bool {
	if _, err := os.Stat(filepath.Join(integerMelangeRepoDir, arch, "APKINDEX.tar.gz")); err != nil {
		return false
	}
	if _, err := os.Stat(integerMelangeKeyPath); err != nil {
		return false
	}
	return true
}

func integerMelangeArch(arch string) string {
	switch arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return arch
	}
}
