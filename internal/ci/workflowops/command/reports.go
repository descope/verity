package command

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowops/reports"
	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func newPushReportsCommand() *cli.Command {
	flags := githubFlags()
	flags = append(
		flags,
		&cli.StringFlag{Name: "branch", Value: "reports"},
		&cli.IntFlag{Name: "attempts", Value: 5},
		&cli.DurationFlag{Name: "base-delay", Value: 5 * time.Second},
		&cli.DurationFlag{Name: "jitter", Value: 5 * time.Second},
		&cli.DurationFlag{Name: "attempt-timeout", Value: 30 * time.Second},
	)
	return &cli.Command{
		Name:      "push-reports",
		Usage:     "Validate and publish JSON report files through the GitHub Contents API",
		ArgsUsage: "REMOTE_PATH LOCAL_FILE [REMOTE_PATH LOCAL_FILE ...]",
		Flags:     flags,
		Action: func(ctx context.Context, command *cli.Command) error {
			if command.Args().Len() < 2 || command.Args().Len()%2 != 0 {
				return fmt.Errorf("%w: push-reports expects remote/local path pairs", ErrInvalidArguments)
			}
			github, repository, err := newGitHubClient(command)
			if err != nil {
				return err
			}
			args := command.Args().Slice()
			files := make([]reports.File, 0, len(args)/2)
			for index := 0; index < len(args); index += 2 {
				files = append(files, reports.File{RemotePath: args[index], LocalPath: args[index+1]})
			}
			pusher := reports.Pusher{
				GitHub: github,
				Engine: retry.Engine{Sleeper: retry.TimerSleeper{}, Random: retry.SystemRandom{}},
				Clock:  systemClock{},
				Stdout: os.Stdout,
			}
			_, err = pusher.Push(ctx, &reports.PushOptions{
				Repository: repository, Branch: command.String("branch"), Files: files,
				Attempts: command.Int("attempts"), BaseDelay: command.Duration("base-delay"), Jitter: command.Duration("jitter"),
				AttemptTimeout: command.Duration("attempt-timeout"),
			})
			return err
		},
	}
}
