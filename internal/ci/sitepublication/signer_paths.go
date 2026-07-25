package sitepublication

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
)

type signerFileIdentity struct {
	device  uint64
	inode   uint64
	nlink   uint64
	mode    uint32
	size    int64
	modTime int64
}

type signerPathRecord struct {
	path     string
	relative string
	identity signerFileIdentity
	digest   publication.Digest
}

type signerPathReference struct {
	name      string
	path      string
	identity  signerFileIdentity
	hasInfo   bool
	directory bool
}

type signerFilesystemState struct {
	snapshot publication.Digest
	packages []string
}

var errSignerPathMissing = errors.New("signer path missing")

func bindSignerFilesystem(plan *SignerPlan) error {
	state, err := inspectSignerFilesystem(plan, false)
	if err != nil {
		return err
	}
	plan.Execution.PathSnapshot = state.snapshot
	return nil
}

func validateSignerFilesystem(plan *SignerPlan, keyMaterialized bool) (signerFilesystemState, error) {
	state, err := inspectSignerFilesystem(plan, keyMaterialized)
	if err != nil {
		return signerFilesystemState{}, err
	}
	if plan.Execution.PathSnapshot == "" || plan.Execution.PathSnapshot != state.snapshot {
		return signerFilesystemState{}, fmt.Errorf("%w: signer host paths changed", ErrInvalidSignerPlan)
	}
	return state, nil
}

func inspectSignerFilesystem(plan *SignerPlan, keyMaterialized bool) (signerFilesystemState, error) {
	if plan == nil {
		return signerFilesystemState{}, fmt.Errorf("%w: signer plan is required", ErrInvalidSignerPlan)
	}
	roots, err := inspectSignerRoots(plan, keyMaterialized)
	if err != nil {
		return signerFilesystemState{}, err
	}
	data, err := inspectSignerData(&plan.Execution)
	if err != nil {
		return signerFilesystemState{}, err
	}
	refs := append(append([]signerPathReference(nil), roots.refs...), data.refs...)
	refs, err = appendSignerModeReferences(refs, data.baseReference, data.deltaReference)
	if err != nil {
		return signerFilesystemState{}, err
	}
	if err := rejectSignerPathOverlap(refs); err != nil {
		return signerFilesystemState{}, err
	}

	snapshot := signerSnapshotDigest(refs, plan.Execution.WorkspaceDir, roots.workspace, roots.keyParent, data.manifestDigest, data.publicKeyDigest, data.deltaDigest, data.packageRecords, data.baseRecords)
	return signerFilesystemState{snapshot: snapshot, packages: data.packagePaths}, nil
}

func validateSignerDirectoryPath(name, path string) (os.FileInfo, error) {
	info, err := lstatSignerPath(path, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidSignerPlan, name, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrInvalidSignerPlan, name)
	}
	return info, nil
}

func validateSignerRegularPath(name, path string) (os.FileInfo, error) {
	info, err := lstatSignerPath(path, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidSignerPlan, name, err)
	}
	if info == nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrInvalidSignerPlan, name)
	}
	identity := signerFileIdentityOf(info)
	if identity.nlink > 1 {
		return nil, fmt.Errorf("%w: %s is hard-linked", ErrInvalidSignerPlan, name)
	}
	return info, nil
}

func lstatSignerPath(path string, allowMissingFinal bool) (os.FileInfo, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("%w: non-canonical absolute path %q", ErrInvalidSignerPlan, path)
	}
	volume := filepath.VolumeName(path)
	root := volume + string(filepath.Separator)
	rest := strings.TrimPrefix(path, root)
	if rest == path {
		rest = strings.TrimPrefix(path, volume)
		rest = strings.TrimPrefix(rest, string(filepath.Separator))
	}
	if rest == "" {
		return os.Lstat(root)
	}
	current := root
	for part := range strings.SplitSeq(rest, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) && allowMissingFinal && current == path {
			return nil, errSignerPathMissing
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: symlink component %q", ErrInvalidSignerPlan, current)
		}
	}
	return os.Lstat(path)
}
