package sitepublication

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/verity-org/verity/internal/ci/publication"
)

var siteFileManifestCodec = canonicalJSONCodec[SiteFileManifest]{
	label: "site file manifest", invalid: ErrUndeclaredMutation, validate: validateSiteFileManifest,
}

func buildSiteFileManifest(root string) (SiteFileManifest, error) {
	files, err := listTreeFiles(root)
	if err != nil {
		return SiteFileManifest{}, err
	}
	manifest := SiteFileManifest{SchemaVersion: SchemaVersion, Files: make([]SiteFile, 0, len(files))}
	for _, file := range files {
		if file.relative == SiteFileManifestPath {
			continue
		}
		digest, err := fileDigest(file.path)
		if err != nil {
			return SiteFileManifest{}, err
		}
		manifest.Files = append(manifest.Files, SiteFile{Path: file.relative, SHA256: digest, Mode: uint32(file.mode.Perm())})
	}
	return manifest, nil
}

func marshalSiteFileManifest(manifest *SiteFileManifest) ([]byte, error) {
	if err := validateSiteFileManifest(manifest); err != nil {
		return nil, err
	}
	canonical := *manifest
	canonical.Files = append([]SiteFile(nil), manifest.Files...)
	sort.Slice(canonical.Files, func(i, j int) bool { return canonical.Files[i].Path < canonical.Files[j].Path })
	return siteFileManifestCodec.marshal(&canonical)
}

func parseSiteFileManifest(data []byte) (SiteFileManifest, error) {
	return siteFileManifestCodec.parse(data)
}

func validateSiteFileManifest(manifest *SiteFileManifest) error {
	if manifest == nil || manifest.SchemaVersion != SchemaVersion || manifest.Files == nil {
		return fmt.Errorf("%w: invalid site file manifest", ErrUndeclaredMutation)
	}
	seen := make(map[string]struct{}, len(manifest.Files))
	previous := ""
	for _, file := range manifest.Files {
		path, err := safeRelative(file.Path)
		if err != nil || path == SiteFileManifestPath || !digestPattern.MatchString(string(file.SHA256)) {
			return fmt.Errorf("%w: invalid file %q", ErrUndeclaredMutation, file.Path)
		}
		if file.Mode != 0o644 && file.Mode != 0o755 {
			return fmt.Errorf("%w: invalid mode for %q", ErrUndeclaredMutation, file.Path)
		}
		if _, exists := seen[path]; exists {
			return fmt.Errorf("%w: duplicate file %q", ErrUndeclaredMutation, path)
		}
		if previous != "" && path <= previous {
			return fmt.Errorf("%w: unsorted file %q", ErrUndeclaredMutation, path)
		}
		seen[path] = struct{}{}
		previous = path
	}
	return nil
}

func VerifySite(root string) (VerifiedSite, error) {
	snapshot, err := captureSiteSnapshot(root)
	if err != nil {
		return VerifiedSite{}, err
	}
	defer snapshot.Close()
	return snapshot.verified, nil
}

func verifySiteTree(root string) (VerifiedSite, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(PublicationManifestPath)))
	if err != nil {
		return VerifiedSite{}, fmt.Errorf("%w: read publication manifest: %w", ErrUndeclaredMutation, err)
	}
	manifest, err := publication.ParseCanonical(manifestBytes)
	if err != nil {
		return VerifiedSite{}, fmt.Errorf("%w: publication manifest: %w", ErrUndeclaredMutation, err)
	}
	declaredBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(SiteFileManifestPath)))
	if err != nil {
		return VerifiedSite{}, fmt.Errorf("%w: read site file manifest: %w", ErrUndeclaredMutation, err)
	}
	declared, err := parseSiteFileManifest(declaredBytes)
	if err != nil {
		return VerifiedSite{}, err
	}
	actual, err := buildSiteFileManifest(root)
	if err != nil {
		return VerifiedSite{}, err
	}
	declaredCanonical, err := marshalSiteFileManifest(&declared)
	if err != nil {
		return VerifiedSite{}, err
	}
	actualCanonical, err := marshalSiteFileManifest(&actual)
	if err != nil {
		return VerifiedSite{}, err
	}
	if !slices.Equal(declaredCanonical, actualCanonical) {
		return VerifiedSite{}, ErrUndeclaredMutation
	}
	manifestDigest, err := publication.DigestManifest(&manifest)
	if err != nil {
		return VerifiedSite{}, err
	}
	siteSum := sha256.Sum256(declaredCanonical)
	return VerifiedSite{
		Manifest: manifest, ManifestDigest: manifestDigest,
		SiteDigest: publication.Digest("sha256:" + hex.EncodeToString(siteSum[:])), FileCount: len(declared.Files),
	}, nil
}
