package sitepublication

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
)

func scanSignerDirectory(root string) ([]signerPathRecord, error) {
	if _, err := validateSignerDirectoryPath("directory", root); err != nil {
		return nil, err
	}
	records := make([]signerPathRecord, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink component %q", ErrInvalidSignerPlan, path)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: unsupported package entry %q", ErrInvalidSignerPlan, path)
		}
		identity := signerFileIdentityOf(info)
		if identity.nlink > 1 {
			return fmt.Errorf("%w: hard-linked package entry %q", ErrInvalidSignerPlan, path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative, err = cleanSignerPath(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		digest, err := signerStableFileDigest(path)
		if err != nil {
			return err
		}
		records = append(records, signerPathRecord{path: path, relative: relative, identity: identity, digest: digest})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: scan %s: %w", ErrInvalidSignerPlan, root, err)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].relative < records[j].relative })
	return records, nil
}

func signerStableFileDigest(path string) (publication.Digest, error) {
	before, err := lstatSignerPath(path, false)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	after, statErr := lstatSignerPath(path, false)
	if err := errors.Join(copyErr, closeErr, statErr); err != nil {
		return "", err
	}
	if !signerSameIdentity(signerFileIdentityOf(before), signerFileIdentityOf(after)) {
		return "", fmt.Errorf("%w: file changed while reading %q", ErrInvalidSignerPlan, path)
	}
	sum := hash.Sum(nil)
	return publication.Digest("sha256:" + hex.EncodeToString(sum)), nil
}

func signerSnapshotDigest(refs []signerPathReference, workspacePath string, workspace, keyParent os.FileInfo, manifest, publicKey, delta publication.Digest, packages, base []signerPathRecord) publication.Digest {
	entries := make([]string, 0, len(refs)+len(packages)+len(base)+4)
	for _, ref := range refs {
		if ref.name == "key directory" {
			continue
		}
		identity := signerIdentityString(ref.identity, ref.hasInfo)
		if ref.directory {
			identity = signerDirectoryIdentityString(ref.identity, ref.hasInfo)
		}
		entries = append(entries, fmt.Sprintf("ref|%s|%s|%s", ref.name, ref.path, identity))
	}
	entries = append(entries,
		"ref|workspace|"+workspacePath+"|"+signerDirectoryIdentityString(signerFileIdentityOf(workspace), true),
		"ref|key parent|"+signerDirectoryIdentityString(signerFileIdentityOf(keyParent), true),
		"file|manifest|"+string(manifest), "file|public-key|"+string(publicKey), "file|delta-manifest|"+string(delta))
	allRecords := append([]signerPathRecord(nil), packages...)
	allRecords = append(allRecords, base...)
	for _, record := range allRecords {
		entries = append(entries, fmt.Sprintf("file|%s|%s|%s", record.path, signerIdentityString(record.identity, true), record.digest))
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return publication.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func signerIdentityString(identity signerFileIdentity, present bool) string {
	if !present {
		return "absent"
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d", identity.device, identity.inode, identity.nlink, identity.mode, identity.size, identity.modTime)
}

func signerDirectoryIdentityString(identity signerFileIdentity, present bool) string {
	if !present {
		return "absent"
	}
	return fmt.Sprintf("%d:%d:%d", identity.device, identity.inode, identity.mode)
}

func signerFileIdentityOf(info os.FileInfo) signerFileIdentity {
	if info == nil {
		return signerFileIdentity{}
	}
	identity := signerFileIdentity{mode: uint32(info.Mode()), size: info.Size(), modTime: info.ModTime().UnixNano()}
	value := reflect.ValueOf(info.Sys())
	if value.IsValid() && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		identity.device = signerUintField(value, "Dev")
		identity.inode = signerUintField(value, "Ino")
		identity.nlink = signerUintField(value, "Nlink")
	}
	return identity
}

func signerUintField(value reflect.Value, name string) uint64 {
	field := value.FieldByName(name)
	if field.IsValid() && field.CanUint() {
		return field.Uint()
	}
	return 0
}

func signerSameIdentity(first, second signerFileIdentity) bool {
	return first.device == second.device && first.inode == second.inode && first.device != 0 && first.inode != 0
}

func signerHostPath(workspace, relative string) string {
	return filepath.Join(workspace, filepath.FromSlash(relative))
}
