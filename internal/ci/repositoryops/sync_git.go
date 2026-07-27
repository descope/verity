package repositoryops

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

type syncRepository struct {
	git  GitRunner
	root string
}

type remoteBranchQuery struct {
	branch       string
	allowMissing bool
}

func (r syncRepository) selectChanges(ctx context.Context, maximum int) (ImageChangeSelection, error) {
	clean, err := r.exit(ctx, []string{"diff", "--cached", "--quiet", "--"}, 0, 1)
	if err != nil {
		return ImageChangeSelection{}, err
	}
	if clean == 1 {
		return ImageChangeSelection{}, fmt.Errorf("%w: staged changes exist before sync", ErrDirtyWorktree)
	}
	status, err := r.required(ctx, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"})
	if err != nil {
		return ImageChangeSelection{}, err
	}
	changes, err := ParseGitStatus(status.Stdout)
	if err != nil {
		return ImageChangeSelection{}, err
	}
	return SelectImageChanges(changes, maximum)
}

func (r syncRepository) ensureFresh(ctx context.Context, request *SyncRequest) error {
	local, err := r.required(ctx, []string{"rev-parse", "HEAD"})
	if err != nil {
		return err
	}
	localOID := strings.TrimSpace(string(local.Stdout))
	if !gitOIDPattern.MatchString(localOID) {
		return fmt.Errorf("%w: local HEAD %q", ErrMalformedOutput, localOID)
	}
	remoteOID, err := r.remoteOID(ctx, remoteBranchQuery{branch: request.baseBranch})
	if err != nil {
		return err
	}
	if localOID != remoteOID {
		return fmt.Errorf("%w: local HEAD %s differs from origin/%s %s", ErrStaleWorktree, localOID, request.baseBranch, remoteOID)
	}
	return nil
}

func (r syncRepository) restoreOverflow(ctx context.Context, changes []FileChange) error {
	tracked := make([]string, 0, len(changes))
	untracked := make([]string, 0, len(changes))
	for _, change := range changes {
		if change.Untracked {
			untracked = append(untracked, change.Path)
		} else {
			tracked = append(tracked, change.Path)
		}
	}
	if len(tracked) > 0 {
		if _, err := r.required(ctx, append([]string{"restore", "--"}, tracked...)); err != nil {
			return fmt.Errorf("restore overflow image definitions: %w", err)
		}
	}
	if len(untracked) > 0 {
		if _, err := r.required(ctx, append([]string{"clean", "-f", "--"}, untracked...)); err != nil {
			return fmt.Errorf("remove overflow image definitions: %w", err)
		}
	}
	return nil
}

func (r syncRepository) stageChanges(ctx context.Context, selection ImageChangeSelection) (SyncResult, bool, error) {
	result := SyncResult{ChangedFiles: changePaths(selection.Selected), RestoredFiles: changePaths(selection.Overflow)}
	if err := r.restoreOverflow(ctx, selection.Overflow); err != nil {
		return SyncResult{}, false, err
	}
	if _, err := r.required(ctx, append([]string{"add", "--"}, result.ChangedFiles...)); err != nil {
		return SyncResult{}, false, err
	}
	staged, err := r.exit(ctx, []string{"diff", "--cached", "--quiet", "--exit-code"}, 0, 1)
	if err != nil {
		return SyncResult{}, false, err
	}
	if staged == 0 {
		result.Unchanged = true
		return result, false, nil
	}
	return result, true, nil
}

func (r syncRepository) commit(ctx context.Context) error {
	commands := [][]string{
		{"config", "user.name", "github-actions[bot]"},
		{"config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"},
		{"commit", "-m", "chore(integer): sync Wolfi package streams"},
	}
	for _, args := range commands {
		if _, err := r.required(ctx, args); err != nil {
			return err
		}
	}
	return nil
}

func (r syncRepository) publish(ctx context.Context, branch string) error {
	if err := r.commit(ctx); err != nil {
		return err
	}
	remoteOID, err := r.remoteOID(ctx, remoteBranchQuery{branch: branch, allowMissing: true})
	if err != nil {
		return err
	}
	lease := "--force-with-lease=refs/heads/" + branch + ":" + remoteOID
	if _, err := r.required(ctx, []string{"push", lease, "origin", "HEAD:refs/heads/" + branch}); err != nil {
		return fmt.Errorf("push sync branch with lease: %w", err)
	}
	return nil
}

func (r syncRepository) remoteOID(ctx context.Context, query remoteBranchQuery) (string, error) {
	result, err := r.git.Run(ctx, GitCommand{Dir: r.root, Args: []string{
		"ls-remote", "--exit-code", "origin", "refs/heads/" + query.branch,
	}})
	if err != nil {
		return "", fmt.Errorf("git ls-remote: %w", err)
	}
	stdout := strings.TrimSpace(string(result.Stdout))
	stderr := strings.TrimSpace(string(result.Stderr))
	if result.ExitCode == 2 && query.allowMissing && stdout == "" && stderr == "" {
		return "", nil
	}
	if result.ExitCode != 0 {
		return "", commandError("git", result)
	}
	fields := strings.Fields(stdout)
	if len(fields) != 2 || fields[1] != "refs/heads/"+query.branch || !gitOIDPattern.MatchString(fields[0]) {
		return "", fmt.Errorf("%w: remote branch %s", ErrMalformedOutput, query.branch)
	}
	return fields[0], nil
}

func (r syncRepository) required(ctx context.Context, args []string) (CommandResult, error) {
	return runGitRequired(ctx, r.git, GitCommand{Dir: r.root, Args: args})
}

func (r syncRepository) exit(ctx context.Context, args []string, allowed ...int) (int, error) {
	result, err := r.git.Run(ctx, GitCommand{Dir: r.root, Args: args})
	if err != nil {
		return 0, fmt.Errorf("run git: %w", err)
	}
	if slices.Contains(allowed, result.ExitCode) {
		return result.ExitCode, nil
	}
	return 0, commandError("git", result)
}

func runGitRequired(ctx context.Context, git GitRunner, request GitCommand) (CommandResult, error) {
	result, err := git.Run(ctx, request)
	if err != nil {
		return CommandResult{}, fmt.Errorf("run git: %w", err)
	}
	if result.ExitCode != 0 {
		return CommandResult{}, commandError("git", result)
	}
	return result, nil
}
