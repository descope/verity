package sitepublication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
)

type overlayMutation struct {
	target string
	source treeFile
}

func AssembleSite(ctx context.Context, request *AssembleRequest) (AssemblyResult, error) {
	if request == nil {
		return AssemblyResult{}, fmt.Errorf("%w: request is required", ErrInvalidAssembly)
	}
	manifestDigest, err := validateAssemblyRequest(ctx, request)
	if err != nil {
		return AssemblyResult{}, err
	}
	stage, err := prepareSiteStage(request.OutputDir)
	if err != nil {
		return AssemblyResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := copyAssemblyInputs(request, stage); err != nil {
		return AssemblyResult{}, err
	}
	fileManifest, fileManifestBytes, err := writeAssemblyContract(request, stage)
	if err != nil {
		return AssemblyResult{}, err
	}
	if err := replaceSiteDirectory(stage, request.OutputDir); err != nil {
		return AssemblyResult{}, err
	}
	committed = true
	sum := sha256.Sum256(fileManifestBytes)
	return AssemblyResult{
		ManifestDigest: manifestDigest,
		SiteDigest:     publication.Digest("sha256:" + hex.EncodeToString(sum[:])),
		FileCount:      len(fileManifest.Files),
	}, nil
}

func validateAssemblyRequest(ctx context.Context, request *AssembleRequest) (publication.Digest, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validatePlan(&request.Plan); err != nil {
		return "", err
	}
	manifestDigest, err := publication.DigestManifest(&request.Manifest)
	if err != nil {
		return "", fmt.Errorf("%w: manifest: %w", ErrInvalidAssembly, err)
	}
	if manifestDigest != request.Plan.ManifestDigest || request.Manifest.Mode != request.Plan.Mode {
		return "", fmt.Errorf("%w: manifest identity", ErrInvalidAssembly)
	}
	if err := validateAssemblyInputs(request); err != nil {
		return "", err
	}
	if request.BaseDir != "" {
		base, err := VerifySite(request.BaseDir)
		if err != nil {
			return "", fmt.Errorf("validate base site: %w", err)
		}
		if base.ManifestDigest != request.Plan.PreviousManifestDigest {
			return "", fmt.Errorf("%w: plan expects %s, site has %s", publication.ErrCASMismatch, request.Plan.PreviousManifestDigest, base.ManifestDigest)
		}
	}
	return manifestDigest, nil
}

func copyAssemblyInputs(request *AssembleRequest, stage string) error {
	if request.BaseDir != "" {
		if err := copyTree(request.BaseDir, stage); err != nil {
			return fmt.Errorf("copy base site: %w", err)
		}
	}
	if err := os.RemoveAll(filepath.Join(stage, "apk")); err != nil {
		return fmt.Errorf("replace APK subtree: %w", err)
	}
	if err := copyTree(request.SignedAPKDir, filepath.Join(stage, "apk")); err != nil {
		return fmt.Errorf("copy signed APK repository: %w", err)
	}
	mutations, err := planOverlays(request.Overlays)
	if err != nil {
		return err
	}
	for _, mutation := range mutations {
		if err := copySiteFile(mutation.source.path, filepath.Join(stage, filepath.FromSlash(mutation.target)), mutation.source.mode); err != nil {
			return err
		}
	}
	return nil
}

func writeAssemblyContract(request *AssembleRequest, stage string) (SiteFileManifest, []byte, error) {
	manifestBytes, err := publication.MarshalCanonical(&request.Manifest)
	if err != nil {
		return SiteFileManifest{}, nil, err
	}
	if err := writeSiteBytes(stage, PublicationManifestPath, manifestBytes, 0o644); err != nil {
		return SiteFileManifest{}, nil, err
	}
	fileManifest, err := buildSiteFileManifest(stage)
	if err != nil {
		return SiteFileManifest{}, nil, err
	}
	fileManifestBytes, err := marshalSiteFileManifest(&fileManifest)
	if err != nil {
		return SiteFileManifest{}, nil, err
	}
	if err := writeSiteBytes(stage, SiteFileManifestPath, fileManifestBytes, 0o644); err != nil {
		return SiteFileManifest{}, nil, err
	}
	return fileManifest, fileManifestBytes, nil
}

func validateAssemblyInputs(request *AssembleRequest) error {
	if request.OutputDir == "" || request.OutputDir == "." || filepath.Clean(request.OutputDir) == string(filepath.Separator) {
		return fmt.Errorf("%w: unsafe output directory", ErrInvalidAssembly)
	}
	switch request.Plan.Mode {
	case publication.ModeBootstrap:
		if request.BaseDir != "" {
			return fmt.Errorf("%w: bootstrap cannot have a base site", ErrInvalidAssembly)
		}
	case publication.ModeSnapshot, publication.ModeDelta:
		if request.BaseDir == "" {
			return fmt.Errorf("%w: base site is required", ErrInvalidAssembly)
		}
	case publication.ModeRestore:
		return fmt.Errorf("%w: restore uses the exact prior artifact", ErrInvalidAssembly)
	}
	if request.SignedAPKDir == "" {
		return fmt.Errorf("%w: signed APK directory is required", ErrInvalidAssembly)
	}
	return nil
}

func planOverlays(overlays []Overlay) ([]overlayMutation, error) {
	ordered := append([]Overlay(nil), overlays...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].Destination < ordered[j].Destination
	})
	owners := make(map[string]string)
	mutations := make([]overlayMutation, 0)
	for _, overlay := range ordered {
		if strings.TrimSpace(overlay.Name) == "" {
			return nil, fmt.Errorf("%w: overlay name is required", ErrInvalidAssembly)
		}
		destination := ""
		if overlay.Destination != "" {
			var err error
			destination, err = safeRelative(filepath.ToSlash(overlay.Destination))
			if err != nil {
				return nil, err
			}
		}
		files, err := listTreeFiles(overlay.SourceDir)
		if err != nil {
			return nil, fmt.Errorf("overlay %q: %w", overlay.Name, err)
		}
		for _, file := range files {
			target := file.relative
			if destination != "" {
				target = filepath.ToSlash(filepath.Join(destination, filepath.FromSlash(file.relative)))
			}
			first, _, _ := strings.Cut(target, "/")
			if first == "apk" || first == ".verity" {
				return nil, fmt.Errorf("%w: overlay %q targets reserved path %q", ErrInvalidAssembly, overlay.Name, target)
			}
			if owner, exists := owners[target]; exists {
				return nil, fmt.Errorf("%w: %s and %s target %s", ErrOverlayConflict, owner, overlay.Name, target)
			}
			owners[target] = overlay.Name
			mutations = append(mutations, overlayMutation{target: target, source: file})
		}
	}
	sort.Slice(mutations, func(i, j int) bool { return mutations[i].target < mutations[j].target })
	return mutations, nil
}
