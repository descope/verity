package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const addImageRemote = "origin"

var ErrConcurrentRemoteChange = errors.New("remote automation branch changed concurrently")

type remoteRefSnapshot struct {
	root      string
	remote    string
	ref       string
	oid       string
	existed   bool
	pushedOID string
}

type remoteRefLocation struct {
	remote string
	ref    string
}

func captureRemoteRef(
	ctx context.Context,
	reader gitStateReader,
	location remoteRefLocation,
) (remoteRefSnapshot, error) {
	oid, existed, err := reader.remoteRef(ctx, location)
	if err != nil {
		return remoteRefSnapshot{}, err
	}
	return remoteRefSnapshot{
		root: reader.root, remote: location.remote, ref: location.ref, oid: oid, existed: existed,
	}, nil
}

func (reader gitStateReader) remoteRef(
	ctx context.Context,
	location remoteRefLocation,
) (oid string, exists bool, err error) {
	result, err := reader.required(ctx, []string{"ls-remote", "--refs", location.remote, location.ref})
	if err != nil {
		return "", false, fmt.Errorf("%w: inspect remote ref %s: %w", ErrGitSnapshot, location.ref, err)
	}
	output := strings.TrimSuffix(string(result.Stdout), "\n")
	if output == "" {
		return "", false, nil
	}
	if strings.Contains(output, "\n") {
		return "", false, fmt.Errorf("%w: multiple values for remote ref %s", ErrGitSnapshot, location.ref)
	}
	parts := strings.Split(output, "\t")
	if len(parts) != 2 || parts[1] != location.ref || !gitOIDPattern.MatchString(parts[0]) {
		return "", false, fmt.Errorf("%w: malformed remote ref output %q", ErrGitSnapshot, output)
	}
	return parts[0], true, nil
}

func (snapshot *remoteRefSnapshot) prepare(ctx context.Context, git GitRunner, localRef string) error {
	oid, exists, err := (gitStateReader{git: git, root: snapshot.root}).optionalRef(ctx, localRef)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: pushed local ref %s is absent", ErrGitSnapshot, localRef)
	}
	snapshot.pushedOID = oid
	return nil
}

func (snapshot *remoteRefSnapshot) restore(ctx context.Context, git GitRunner) error {
	if snapshot.pushedOID == "" {
		return nil
	}
	reader := gitStateReader{git: git, root: snapshot.root}
	currentOID, currentExists, err := reader.remoteRef(ctx, remoteRefLocation{remote: snapshot.remote, ref: snapshot.ref})
	if err != nil {
		return err
	}
	if sameRefState(snapshot.oid, snapshot.existed, currentOID, currentExists) {
		return nil
	}
	if !currentExists || currentOID != snapshot.pushedOID {
		return fmt.Errorf(
			"%w: %s expected transaction OID %s, found %s",
			ErrConcurrentRemoteChange,
			snapshot.ref,
			snapshot.pushedOID,
			formatRefState(currentOID, currentExists),
		)
	}
	lease := "--force-with-lease=" + snapshot.ref + ":" + snapshot.pushedOID
	source := ""
	if snapshot.existed {
		source = snapshot.oid
	}
	_, err = runGitRequired(ctx, git, GitCommand{
		Dir:  snapshot.root,
		Args: []string{"push", lease, snapshot.remote, source + ":" + snapshot.ref},
	})
	if err != nil {
		return fmt.Errorf("restore remote ref %s: %w", snapshot.ref, err)
	}
	return nil
}

func sameRefState(firstOID string, firstExists bool, secondOID string, secondExists bool) bool {
	return firstExists == secondExists && (!firstExists || firstOID == secondOID)
}

func formatRefState(oid string, exists bool) string {
	if !exists {
		return "absent"
	}
	return oid
}
