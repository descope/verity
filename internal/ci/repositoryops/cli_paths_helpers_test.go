package repositoryops

import "context"

type commandRunnerFunc func(context.Context, *Command) (CommandResult, error)

func (run commandRunnerFunc) Run(ctx context.Context, command *Command) (CommandResult, error) {
	return run(ctx, command)
}

type patcherFunc func(context.Context, *PatchSpec) error

func (patch patcherFunc) Patch(ctx context.Context, spec *PatchSpec) error {
	return patch(ctx, spec)
}

type gitRunnerFunc func(context.Context, GitCommand) (CommandResult, error)

func (run gitRunnerFunc) Run(ctx context.Context, command GitCommand) (CommandResult, error) {
	return run(ctx, command)
}

type gitHubRunnerFunc func(context.Context, GitHubCommand) (CommandResult, error)

func (run gitHubRunnerFunc) Run(ctx context.Context, command GitHubCommand) (CommandResult, error) {
	return run(ctx, command)
}

func commandFlagValue(arguments []string, name string) string {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == name {
			return arguments[index+1]
		}
	}
	return ""
}
