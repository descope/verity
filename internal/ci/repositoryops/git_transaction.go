package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const addImageRollbackTimeout = 30 * time.Second

var (
	ErrGitSnapshot = errors.New("capture Git repository state")
	ErrGitRollback = errors.New("restore Git repository state")
)

type addImageTransactionSnapshot struct {
	git      *gitRepositorySnapshot
	worktree worktreeSnapshot
}

type gitRepositorySnapshot struct {
	root                 string
	headOID              string
	symbolicRef          string
	automationRef        string
	automationOID        string
	automationRefExisted bool
	trackingRef          string
	trackingOID          string
	trackingRefExisted   bool
	remote               remoteRefSnapshot
	head                 gitFileSnapshot
	index                gitFileSnapshot
	config               gitFileSnapshot
	locks                gitLockSnapshot
}

type gitStateReader struct {
	git  GitRunner
	root string
}

func captureAddImageTransaction(
	ctx context.Context,
	git GitRunner,
	request *AddImageRequest,
) (*addImageTransactionSnapshot, error) {
	gitSnapshot, err := captureGitRepository(ctx, gitStateReader{git: git, root: request.repoRoot}, "refs/heads/"+request.branch)
	if err != nil {
		return nil, err
	}
	worktree, err := captureWorktree(request.repoRoot)
	if err != nil {
		return nil, err
	}
	return &addImageTransactionSnapshot{git: gitSnapshot, worktree: worktree}, nil
}

func captureGitRepository(ctx context.Context, reader gitStateReader, automationRef string) (*gitRepositorySnapshot, error) {
	headResult, err := reader.required(ctx, []string{"rev-parse", "--verify", "HEAD"})
	if err != nil {
		return nil, fmt.Errorf("%w: HEAD: %w", ErrGitSnapshot, err)
	}
	headOID := strings.TrimSpace(string(headResult.Stdout))
	if !gitOIDPattern.MatchString(headOID) {
		return nil, fmt.Errorf("%w: malformed HEAD OID %q", ErrGitSnapshot, headOID)
	}
	symbolicRef, err := reader.symbolicHEAD(ctx)
	if err != nil {
		return nil, err
	}
	automationOID, automationRefExisted, err := reader.optionalRef(ctx, automationRef)
	if err != nil {
		return nil, err
	}
	if automationRefExisted {
		return nil, fmt.Errorf("%w: %s", ErrStaleAutomationBranch, strings.TrimPrefix(automationRef, "refs/heads/"))
	}
	branch := strings.TrimPrefix(automationRef, "refs/heads/")
	trackingRef := "refs/remotes/" + addImageRemote + "/" + branch
	trackingOID, trackingRefExisted, err := reader.optionalRef(ctx, trackingRef)
	if err != nil {
		return nil, err
	}
	remote, err := captureRemoteRef(ctx, reader, remoteRefLocation{remote: addImageRemote, ref: automationRef})
	if err != nil {
		return nil, err
	}
	head, err := reader.gitFile(ctx, "HEAD", true)
	if err != nil {
		return nil, err
	}
	index, err := reader.gitFile(ctx, "index", true)
	if err != nil {
		return nil, err
	}
	config, err := reader.gitFile(ctx, "config", true)
	if err != nil {
		return nil, err
	}
	gitDir, err := reader.gitDirectory(ctx, "--git-dir")
	if err != nil {
		return nil, err
	}
	commonDir, err := reader.gitDirectory(ctx, "--git-common-dir")
	if err != nil {
		return nil, err
	}
	lockRoots, err := gitDirectoryRoots(gitDir, commonDir)
	if err != nil {
		return nil, err
	}
	locks, err := captureGitLocks(lockRoots)
	if err != nil {
		return nil, err
	}
	return &gitRepositorySnapshot{
		root: reader.root, headOID: headOID, symbolicRef: symbolicRef,
		automationRef: automationRef, automationOID: automationOID, automationRefExisted: automationRefExisted,
		trackingRef: trackingRef, trackingOID: trackingOID, trackingRefExisted: trackingRefExisted,
		remote: remote,
		head:   head, index: index, config: config,
		locks: locks,
	}, nil
}

func (reader gitStateReader) symbolicHEAD(ctx context.Context) (string, error) {
	result, err := reader.git.Run(ctx, GitCommand{Dir: reader.root, Args: []string{"symbolic-ref", "-q", "HEAD"}})
	if err != nil {
		return "", fmt.Errorf("%w: read symbolic HEAD: %w", ErrGitSnapshot, err)
	}
	value := strings.TrimSpace(string(result.Stdout))
	if result.ExitCode == 1 && value == "" && len(result.Stderr) == 0 {
		return "", nil
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("%w: %w", ErrGitSnapshot, commandError("git symbolic-ref", result))
	}
	if !strings.HasPrefix(value, "refs/heads/") {
		return "", fmt.Errorf("%w: malformed symbolic HEAD %q", ErrGitSnapshot, value)
	}
	if _, err := validatedBranch(strings.TrimPrefix(value, "refs/heads/")); err != nil {
		return "", fmt.Errorf("%w: symbolic HEAD: %w", ErrGitSnapshot, err)
	}
	return value, nil
}

func (reader gitStateReader) optionalRef(ctx context.Context, ref string) (oid string, exists bool, err error) {
	result, err := reader.git.Run(ctx, GitCommand{Dir: reader.root, Args: []string{"rev-parse", "--verify", "--quiet", ref}})
	if err != nil {
		return "", false, fmt.Errorf("%w: inspect ref %s: %w", ErrGitSnapshot, ref, err)
	}
	value := strings.TrimSpace(string(result.Stdout))
	if result.ExitCode == 1 && value == "" && len(result.Stderr) == 0 {
		return "", false, nil
	}
	if result.ExitCode != 0 {
		return "", false, fmt.Errorf("%w: %w", ErrGitSnapshot, commandError("git rev-parse", result))
	}
	if !gitOIDPattern.MatchString(value) {
		return "", false, fmt.Errorf("%w: malformed ref OID %q", ErrGitSnapshot, value)
	}
	return value, true, nil
}

func (reader gitStateReader) gitFile(ctx context.Context, name string, required bool) (gitFileSnapshot, error) {
	result, err := reader.required(ctx, []string{"rev-parse", "--path-format=absolute", "--git-path", name})
	if err != nil {
		return gitFileSnapshot{}, fmt.Errorf("%w: locate %s: %w", ErrGitSnapshot, name, err)
	}
	path := strings.TrimSpace(string(result.Stdout))
	snapshot, err := captureGitFile(gitFileSnapshotRequest{path: path, label: name, required: required})
	if err != nil {
		return gitFileSnapshot{}, fmt.Errorf("%w: %w", ErrGitSnapshot, err)
	}
	return snapshot, nil
}

func (reader gitStateReader) gitDirectory(ctx context.Context, argument string) (string, error) {
	result, err := reader.required(ctx, []string{"rev-parse", "--path-format=absolute", argument})
	if err != nil {
		return "", fmt.Errorf("%w: locate %s: %w", ErrGitSnapshot, argument, err)
	}
	path := strings.TrimSpace(string(result.Stdout))
	if path == "" {
		return "", fmt.Errorf("%w: empty %s path", ErrGitSnapshot, argument)
	}
	return path, nil
}

func (reader gitStateReader) required(ctx context.Context, args []string) (CommandResult, error) {
	return runGitRequired(ctx, reader.git, GitCommand{Dir: reader.root, Args: args})
}

func (snapshot *addImageTransactionSnapshot) restore(ctx context.Context, git GitRunner) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), addImageRollbackTimeout)
	defer cancel()
	gitErr := snapshot.git.restore(rollbackCtx, git)
	worktreeErr := snapshot.worktree.restore()
	return errors.Join(gitErr, worktreeErr)
}

func (snapshot *gitRepositorySnapshot) restore(ctx context.Context, git GitRunner) error {
	var restoreErrors []error
	if _, err := snapshot.locks.removeCreated(); err != nil {
		restoreErrors = append(restoreErrors, err)
	}
	if err := snapshot.remote.restore(ctx, git); err != nil {
		restoreErrors = append(restoreErrors, err)
	}
	if err := snapshot.restoreRefs(ctx, git); err != nil {
		removed, lockErr := snapshot.locks.removeCreated()
		restoreErrors = append(restoreErrors, lockErr)
		if !removed {
			restoreErrors = append(restoreErrors, err)
		} else if retryErr := snapshot.restoreRefs(ctx, git); retryErr != nil {
			restoreErrors = append(restoreErrors, retryErr)
		}
	}
	for _, file := range []gitFileSnapshot{snapshot.head, snapshot.index, snapshot.config} {
		if err := file.restore(); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
	}
	if _, err := snapshot.locks.removeCreated(); err != nil {
		restoreErrors = append(restoreErrors, err)
	}
	if err := snapshot.locks.restoreExisting(); err != nil {
		restoreErrors = append(restoreErrors, err)
	}
	if err := errors.Join(restoreErrors...); err != nil {
		return fmt.Errorf("%w: %w", ErrGitRollback, err)
	}
	return nil
}
