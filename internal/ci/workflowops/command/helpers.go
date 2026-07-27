package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
	"github.com/verity-org/verity/internal/ci/workflowops/producer"
	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

var (
	ErrInvalidArguments    = errors.New("invalid workflowops arguments")
	ErrGitHubTokenRequired = errors.New("GitHub token is required")
	ErrPositiveInteger     = errors.New("positive integer required")
	ErrRetryPolicy         = errors.New("invalid command retry policy")
)

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

func githubFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "repository", Usage: "GitHub repository (owner/name)", Sources: cli.EnvVars("GITHUB_REPOSITORY")},
		&cli.StringFlag{Name: "github-token", Usage: "GitHub API token", Hidden: true, Sources: cli.EnvVars("GH_TOKEN")},
		&cli.StringFlag{Name: "github-api-url", Usage: "GitHub API base URL", Value: "https://api.github.com"},
	}
}

func newGitHubClient(command *cli.Command) (*githubapi.Client, githubapi.Repository, error) {
	repository, err := githubapi.NewRepository(command.String("repository"))
	if err != nil {
		return nil, githubapi.Repository{}, err
	}
	if command.String("github-token") == "" {
		return nil, githubapi.Repository{}, ErrGitHubTokenRequired
	}
	runner, err := githubapi.NewHTTPRunner(command.String("github-api-url"), command.String("github-token"))
	if err != nil {
		return nil, githubapi.Repository{}, err
	}
	return &githubapi.Client{Runner: runner}, repository, nil
}

func parsePositiveInt64(label, value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%w: %s", ErrPositiveInteger, label)
	}
	return parsed, nil
}

func retryProcess() *retry.Process {
	return &retry.Process{
		Runner: retry.ExecRunner{},
		Engine: retry.Engine{Sleeper: retry.TimerSleeper{}, Random: retry.SystemRandom{}},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

func retryExitError(err error) error {
	var commandErr *retry.CommandError
	if errors.As(err, &commandErr) && commandErr.ExitCode > 0 {
		return cli.Exit(err, commandErr.ExitCode)
	}
	return err
}

func appendOutputs(path string, outputs []producer.Output) (retErr error) {
	if path == "" || len(outputs) == 0 {
		return nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open GitHub output %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close GitHub output %q: %w", path, closeErr))
		}
	}()
	for _, output := range outputs {
		if _, err := fmt.Fprintf(file, "%s=%s\n", output.Name, output.Value); err != nil {
			return fmt.Errorf("write GitHub output %q: %w", path, err)
		}
	}
	return nil
}

func openAppend(path string) (io.Closer, io.Writer, error) {
	if path == "" {
		return nil, nil, nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open append target %q: %w", path, err)
	}
	return file, file, nil
}

func closeWithError(err error, closer io.Closer, label string) error {
	if closer == nil {
		return err
	}
	if closeErr := closer.Close(); closeErr != nil {
		return errors.Join(err, fmt.Errorf("close %s: %w", label, closeErr))
	}
	return err
}
