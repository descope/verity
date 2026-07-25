package command

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	workflowretry "github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func retryFlags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{Name: "attempts", Value: 4, Sources: cli.EnvVars("REGISTRY_COMMAND_ATTEMPTS")},
		&cli.IntFlag{Name: "base-delay-seconds", Value: 10, Sources: cli.EnvVars("REGISTRY_COMMAND_BASE_DELAY_SECONDS")},
		&cli.IntFlag{Name: "jitter-seconds", Value: 10},
		&cli.DurationFlag{Name: "timeout", Usage: "Per-attempt timeout; zero disables the deadline"},
	}
}

func newRetryCommand() *cli.Command {
	return &cli.Command{
		Name:      "retry-command",
		Usage:     "Run a command with bounded linear backoff",
		ArgsUsage: "LABEL COMMAND [ARG ...]",
		Flags:     retryFlags(),
		Action: func(ctx context.Context, command *cli.Command) error {
			if command.Args().Len() < 2 {
				return fmt.Errorf("%w: retry-command expects a label and command", ErrInvalidArguments)
			}
			args := command.Args().Slice()
			policy, err := registryPolicy(command)
			if err != nil {
				return err
			}
			operation := workflowretry.Operation{
				Label: args[0],
				Command: workflowretry.Command{
					Name: args[1], Args: args[2:], Timeout: command.Duration("timeout"),
				},
			}
			err = retryProcess().Run(ctx, &operation, policy)
			return retryExitError(err)
		},
	}
}

func newRetryGoBuildCommand() *cli.Command {
	return &cli.Command{
		Name:  "retry-go-build",
		Usage: "Build the verity binary with registry retry policy",
		Flags: retryFlags(),
		Action: func(ctx context.Context, command *cli.Command) error {
			policy, err := registryPolicy(command)
			if err != nil {
				return err
			}
			operation := workflowretry.Operation{
				Label: "go build verity",
				Command: workflowretry.Command{
					Name: "go", Args: []string{"build", "-o", "verity", "."}, Timeout: command.Duration("timeout"),
				},
			}
			err = retryProcess().Run(ctx, &operation, policy)
			return retryExitError(err)
		},
	}
}

func newRetryDockerLoginCommand() *cli.Command {
	return &cli.Command{
		Name:  "retry-docker-login",
		Usage: "Log in to a registry with timeout and retry policy",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "registry", Sources: cli.EnvVars("DOCKER_REGISTRY")},
			&cli.StringFlag{Name: "username", Sources: cli.EnvVars("DOCKER_USERNAME")},
			&cli.StringFlag{Name: "password", Hidden: true, Sources: cli.EnvVars("DOCKER_PASSWORD")},
			&cli.IntFlag{Name: "attempts", Value: 4, Sources: cli.EnvVars("DOCKER_LOGIN_ATTEMPTS")},
			&cli.IntFlag{Name: "timeout-seconds", Value: 45, Sources: cli.EnvVars("DOCKER_LOGIN_TIMEOUT_SECONDS")},
			&cli.IntFlag{Name: "base-delay-seconds", Value: 10},
			&cli.IntFlag{Name: "jitter-seconds", Value: 10},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			operation, policy, err := workflowretry.NewDockerLoginOperation(&workflowretry.DockerLoginOptions{
				Registry: command.String("registry"), Username: command.String("username"), Password: command.String("password"),
				Attempts: command.Int("attempts"), Timeout: time.Duration(command.Int("timeout-seconds")) * time.Second,
				BaseDelay: time.Duration(command.Int("base-delay-seconds")) * time.Second,
				Jitter:    time.Duration(command.Int("jitter-seconds")) * time.Second,
			})
			if err != nil {
				return err
			}
			return retryExitError(retryProcess().Run(ctx, &operation, policy))
		},
	}
}

func registryPolicy(command *cli.Command) (workflowretry.Policy, error) {
	attempts := command.Int("attempts")
	baseSeconds := command.Int("base-delay-seconds")
	jitterSeconds := command.Int("jitter-seconds")
	if attempts < 1 || baseSeconds < 1 || jitterSeconds < 0 {
		return workflowretry.Policy{}, ErrRetryPolicy
	}
	return workflowretry.Policy{
		MaxAttempts: attempts,
		BaseDelay:   time.Duration(baseSeconds) * time.Second,
		Jitter:      time.Duration(jitterSeconds) * time.Second,
	}, nil
}
