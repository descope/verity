package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func TestIntegerPrepareMelangeBuild_RunsScriptAndReturnsRepoAndKey(t *testing.T) {
	repoRoot := t.TempDir()
	scriptPath := filepath.Join(repoRoot, "melange-build.sh")
	envCapturePath := filepath.Join(repoRoot, "env.txt")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nset -eu\nprintf '%s\\n%s\\n%s\\n%s\\n' \"$BESPOKE_JSON\" \"$UPSTREAM\" \"$ENV_FILE\" \"$BUILD_OPTION\" > \""+envCapturePath+"\"\nmkdir -p packages/repo melange-work\n: > melange-work/melange.rsa.pub\n"), 0o755))

	originalScriptPath := integerMelangeBuildScriptPath
	integerMelangeBuildScriptPath = scriptPath
	t.Cleanup(func() { integerMelangeBuildScriptPath = originalScriptPath })

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoRoot))
	t.Cleanup(func() { require.NoError(t, os.Chdir(wd)) })

	repos, keyrings, err := integerPrepareMelangeBuild(context.Background(), &intconfig.MelangeSpec{
		Bespoke:     intconfig.StringList{"custom.yaml"},
		EnvFile:     "fips.env",
		BuildOption: "fips",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{integerMelangeRepoDir}, repos)
	assert.Equal(t, []string{integerMelangeKeyPath}, keyrings)

	data, err := os.ReadFile(envCapturePath)
	require.NoError(t, err)
	assert.Equal(t, "[\"custom.yaml\"]\n\nfips.env\nfips\n", string(data))
}

func TestIntegerRunApkoBuild_AppendsExtraRepositoriesAndKeyrings(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "apko")
	body := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > \"" + argsPath + "\"\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	err := integerRunApkoBuild(
		context.Background(),
		"config.yaml",
		"out.tar",
		"amd64",
		[]string{"packages/repo"},
		[]string{"melange-work/melange.rsa.pub"},
	)
	require.NoError(t, err)

	data, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Equal(t, "build\n--arch\namd64\n--repository-append\npackages/repo\n--keyring-append\nmelange-work/melange.rsa.pub\nconfig.yaml\ninteger:local\nout.tar\n", string(data))
}
