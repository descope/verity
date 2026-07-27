package repositoryops

import (
	"io"
	"os"
	"strconv"

	"github.com/urfave/cli/v3"
)

type cliDependencies struct {
	commands CommandRunner
	patcher  Patcher
	git      GitRunner
	github   func(string) (GitHubRunner, error)
	stdout   io.Writer
	getenv   func(string) string
}

func NewCLICommand() *cli.Command {
	commands := ExecCommandRunner{}
	deps := cliDependencies{
		commands: commands,
		patcher:  CopaPatcher{},
		git:      NewGitRunner(commands),
		github:   func(token string) (GitHubRunner, error) { return NewGitHubRunner(commands, token) },
		stdout:   os.Stdout,
		getenv:   os.Getenv,
	}
	return newCLICommand(&deps)
}

func newCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name:  "repository-ops",
		Usage: "Run typed repository automation formerly implemented in shell",
		Commands: []*cli.Command{
			newPatchImageCLICommand(deps),
			newScanBeforeCLICommand(deps),
			newVerifyPatchedCLICommand(deps),
			newCatalogEntryCLICommand(deps),
			newNativePackageCLICommand(deps),
			newSealedSecretsImageCLICommand(deps),
			newParseImageIssueCLICommand(deps),
			newAddStandaloneImageCLICommand(deps),
			newSyncPullRequestCLICommand(deps),
		},
	}
}

func countWorkflowValues(prefix string, counts VulnerabilityCounts) []WorkflowValue {
	return []WorkflowValue{
		{Name: prefix + "_total", Value: strconv.Itoa(counts.Total)},
		{Name: prefix + "_go", Value: strconv.Itoa(counts.Go)},
		{Name: prefix + "_non_go", Value: strconv.Itoa(counts.NonGo)},
	}
}

func appendCLIWorkflowValues(flagPath, environmentPath string, values []WorkflowValue) error {
	path := flagPath
	if path == "" {
		path = environmentPath
	}
	if path == "" {
		return nil
	}
	return AppendWorkflowValues(path, values)
}

func boolExitCode(err error) int {
	if err != nil {
		return 1
	}
	return 0
}
