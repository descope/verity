package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrConcurrentLocalChange = errors.New("local Git ref changed concurrently")

type refRestore struct {
	ref            string
	oid            string
	existed        bool
	currentOID     string
	currentExists  bool
	transactionOID string
	fallbackOID    string
}

func (snapshot *gitRepositorySnapshot) restoreRefs(ctx context.Context, git GitRunner) error {
	reader := gitStateReader{git: git, root: snapshot.root}
	automationOID, automationExists, err := reader.optionalRef(ctx, snapshot.automationRef)
	if err != nil {
		return err
	}
	trackingOID, trackingExists, err := reader.optionalRef(ctx, snapshot.trackingRef)
	if err != nil {
		return err
	}
	var commands strings.Builder
	commands.WriteString("start\n")
	if err := appendRefRestore(&commands, &refRestore{
		ref: snapshot.automationRef, oid: snapshot.automationOID, existed: snapshot.automationRefExisted,
		currentOID: automationOID, currentExists: automationExists,
		transactionOID: snapshot.remote.pushedOID, fallbackOID: snapshot.headOID,
	}); err != nil {
		return err
	}
	if err := appendRefRestore(&commands, &refRestore{
		ref: snapshot.trackingRef, oid: snapshot.trackingOID, existed: snapshot.trackingRefExisted,
		currentOID: trackingOID, currentExists: trackingExists, transactionOID: snapshot.remote.pushedOID,
	}); err != nil {
		return err
	}
	commands.WriteString("prepare\ncommit\n")
	_, err = runGitRequired(ctx, git, GitCommand{
		Dir: snapshot.root, Args: []string{"update-ref", "--stdin"}, Stdin: []byte(commands.String()),
	})
	if err != nil {
		return fmt.Errorf("restore refs transaction: %w", err)
	}
	return nil
}

func appendRefRestore(commands *strings.Builder, state *refRestore) error {
	if sameRefState(state.oid, state.existed, state.currentOID, state.currentExists) {
		return nil
	}
	if !state.currentExists || (state.currentOID != state.transactionOID && state.currentOID != state.fallbackOID) {
		return fmt.Errorf("%w: %s", ErrConcurrentLocalChange, state.ref)
	}
	if state.existed {
		fmt.Fprintf(commands, "update %s %s %s\n", state.ref, state.oid, state.currentOID)
	} else {
		fmt.Fprintf(commands, "delete %s %s\n", state.ref, state.currentOID)
	}
	return nil
}
