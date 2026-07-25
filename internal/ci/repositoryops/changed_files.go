package repositoryops

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"
)

var (
	ErrMalformedGitStatus = errors.New("malformed git status output")
	ErrInvalidChangedPath = errors.New("invalid changed file path")
	ErrDirtyWorktree      = errors.New("worktree contains unsupported changes")
	ErrInvalidChangeLimit = errors.New("maximum changed images must be positive")
)

type FileChange struct {
	Path      string
	Index     byte
	Worktree  byte
	Untracked bool
}

func ParseGitStatus(output []byte) ([]FileChange, error) {
	if len(output) == 0 {
		return nil, nil
	}
	if output[len(output)-1] != 0 {
		return nil, fmt.Errorf("%w: output is not NUL terminated", ErrMalformedGitStatus)
	}
	records := strings.Split(string(output[:len(output)-1]), "\x00")
	changes := make([]FileChange, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if len(record) < 4 || record[2] != ' ' {
			return nil, fmt.Errorf("%w: invalid record", ErrMalformedGitStatus)
		}
		index, worktree := record[0], record[1]
		if index == 'R' || index == 'C' || worktree == 'R' || worktree == 'C' {
			return nil, fmt.Errorf("%w: renamed and copied files are not supported", ErrDirtyWorktree)
		}
		changedPath := record[3:]
		if err := validateChangedPath(changedPath); err != nil {
			return nil, err
		}
		if _, ok := seen[changedPath]; ok {
			return nil, fmt.Errorf("%w: duplicate path %q", ErrMalformedGitStatus, changedPath)
		}
		seen[changedPath] = struct{}{}
		changes = append(changes, FileChange{
			Path: changedPath, Index: index, Worktree: worktree, Untracked: index == '?' && worktree == '?',
		})
	}
	return changes, nil
}

type ImageChangeSelection struct {
	Selected []FileChange
	Overflow []FileChange
}

func SelectImageChanges(changes []FileChange, maximum int) (ImageChangeSelection, error) {
	if maximum <= 0 {
		return ImageChangeSelection{}, ErrInvalidChangeLimit
	}
	selected := make([]FileChange, 0, len(changes))
	for _, change := range changes {
		if change.Index != ' ' && change.Index != '?' {
			return ImageChangeSelection{}, fmt.Errorf("%w: staged path %q", ErrDirtyWorktree, change.Path)
		}
		if change.Index == 'D' || change.Worktree == 'D' {
			return ImageChangeSelection{}, fmt.Errorf("%w: deleted path %q", ErrDirtyWorktree, change.Path)
		}
		if !strings.HasPrefix(change.Path, "images/") || path.Ext(change.Path) != ".yaml" {
			return ImageChangeSelection{}, fmt.Errorf("%w: out-of-scope path %q", ErrDirtyWorktree, change.Path)
		}
		selected = append(selected, change)
	}
	slices.SortFunc(selected, func(left, right FileChange) int {
		return strings.Compare(left.Path, right.Path)
	})
	if len(selected) <= maximum {
		return ImageChangeSelection{Selected: selected}, nil
	}
	return ImageChangeSelection{Selected: selected[:maximum], Overflow: selected[maximum:]}, nil
}

func validateChangedPath(value string) error {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return fmt.Errorf("%w: %q", ErrInvalidChangedPath, value)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: path contains control characters", ErrInvalidChangedPath)
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%w: %q", ErrInvalidChangedPath, value)
	}
	return nil
}
