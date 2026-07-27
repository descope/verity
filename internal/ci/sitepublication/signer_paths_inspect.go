package sitepublication

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/verity-org/verity/internal/ci/publication"
)

type signerRootState struct {
	refs      []signerPathReference
	workspace os.FileInfo
	keyParent os.FileInfo
}

func inspectSignerRoots(plan *SignerPlan, keyMaterialized bool) (signerRootState, error) {
	spec := &plan.Execution
	workspace, err := validateSignerDirectoryPath("workspace", spec.WorkspaceDir)
	if err != nil {
		return signerRootState{}, err
	}
	keyParent, err := validateSignerDirectoryPath("key parent", filepath.Dir(plan.Cleanup.KeyDirectory))
	if err != nil {
		return signerRootState{}, err
	}
	keyInfo, err := lstatSignerPath(plan.Cleanup.KeyDirectory, true)
	if errors.Is(err, errSignerPathMissing) {
		err = nil
	}
	if err != nil {
		return signerRootState{}, fmt.Errorf("%w: key directory: %w", ErrInvalidSignerPlan, err)
	}
	if keyMaterialized {
		if err := validateMaterializedSignerKeyDirectory(plan, workspace, keyInfo); err != nil {
			return signerRootState{}, err
		}
	} else if keyInfo != nil {
		return signerRootState{}, fmt.Errorf("%w: signer key directory already exists", ErrInvalidSignerPlan)
	}
	return signerRootState{workspace: workspace, keyParent: keyParent, refs: []signerPathReference{{name: "key directory", path: plan.Cleanup.KeyDirectory, hasInfo: keyInfo != nil, identity: signerFileIdentityOf(keyInfo)}}}, nil
}

func validateMaterializedSignerKeyDirectory(plan *SignerPlan, workspace, keyInfo os.FileInfo) error {
	if keyInfo == nil || !keyInfo.IsDir() {
		return fmt.Errorf("%w: signer key directory is missing", ErrInvalidSignerPlan)
	}
	if signerSameIdentity(signerFileIdentityOf(workspace), signerFileIdentityOf(keyInfo)) {
		return fmt.Errorf("%w: signer key directory aliases workspace", ErrInvalidSignerPlan)
	}
	_, err := validateSignerRegularPath("signer key", plan.Cleanup.KeyPath)
	return err
}

type signerDataState struct {
	manifestDigest  publication.Digest
	publicKeyDigest publication.Digest
	deltaDigest     publication.Digest
	packageRecords  []signerPathRecord
	baseRecords     []signerPathRecord
	packagePaths    []string
	baseReference   *signerPathReference
	deltaReference  *signerPathReference
	refs            []signerPathReference
}

func inspectSignerData(spec *SignerExecutionSpec) (signerDataState, error) {
	manifestPath := signerHostPath(spec.WorkspaceDir, spec.ManifestPath)
	publicKeyPath := signerHostPath(spec.WorkspaceDir, spec.PublicKeyPath)
	packagesPath := signerHostPath(spec.WorkspaceDir, spec.PackagesPath)
	outputPath := signerHostPath(spec.WorkspaceDir, spec.OutputAPKPath)
	manifest, err := validateSignerRegularPath("manifest", manifestPath)
	if err != nil {
		return signerDataState{}, err
	}
	publicKey, err := validateSignerRegularPath("public key", publicKeyPath)
	if err != nil {
		return signerDataState{}, err
	}
	packages, err := validateSignerDirectoryPath("packages", packagesPath)
	if err != nil {
		return signerDataState{}, err
	}
	output, err := validateSignerDirectoryPath("output", outputPath)
	if err != nil {
		return signerDataState{}, err
	}
	manifestDigest, err := signerStableFileDigest(manifestPath)
	if err != nil {
		return signerDataState{}, err
	}
	publicKeyDigest, err := signerStableFileDigest(publicKeyPath)
	if err != nil {
		return signerDataState{}, err
	}
	packageRecords, err := scanSignerDirectory(packagesPath)
	if err != nil {
		return signerDataState{}, err
	}
	if _, err := scanSignerDirectory(outputPath); err != nil {
		return signerDataState{}, err
	}
	packagePaths := make([]string, 0, len(packageRecords))
	for _, record := range packageRecords {
		if record.relative == "APKINDEX.tar.gz" {
			return signerDataState{}, fmt.Errorf("%w: packages contain reserved APKINDEX.tar.gz", ErrInvalidSignerPlan)
		}
		packagePaths = append(packagePaths, record.relative)
	}
	if len(packagePaths) == 0 {
		return signerDataState{}, fmt.Errorf("%w: packages directory is empty", ErrInvalidSignerPlan)
	}
	data := signerDataState{
		manifestDigest: manifestDigest, publicKeyDigest: publicKeyDigest,
		packageRecords: packageRecords, packagePaths: packagePaths,
	}
	data.refs = []signerPathReference{
		{name: "manifest", path: manifestPath, identity: signerFileIdentityOf(manifest), hasInfo: true},
		{name: "packages", path: packagesPath, identity: signerFileIdentityOf(packages), hasInfo: true, directory: true},
		{name: "output", path: outputPath, identity: signerFileIdentityOf(output), hasInfo: true, directory: true},
		{name: "public key", path: publicKeyPath, identity: signerFileIdentityOf(publicKey), hasInfo: true},
	}
	data.baseReference, data.deltaReference, data.baseRecords, data.deltaDigest, err = inspectSignerDelta(spec)
	if err != nil {
		return signerDataState{}, err
	}
	return data, nil
}

func inspectSignerDelta(spec *SignerExecutionSpec) (baseReference, deltaReference *signerPathReference, baseRecords []signerPathRecord, deltaDigest publication.Digest, err error) {
	if spec.Mode != publication.ModeDelta {
		return nil, nil, nil, "", nil
	}
	basePath := signerHostPath(spec.WorkspaceDir, spec.BaseAPKPath)
	deltaPath := signerHostPath(spec.WorkspaceDir, spec.DeltaManifestPath)
	baseInfo, err := validateSignerDirectoryPath("base APK", basePath)
	if err != nil {
		return nil, nil, nil, "", err
	}
	deltaInfo, err := validateSignerRegularPath("delta manifest", deltaPath)
	if err != nil {
		return nil, nil, nil, "", err
	}
	baseRecords, err = scanSignerDirectory(basePath)
	if err != nil {
		return nil, nil, nil, "", err
	}
	deltaDigest, err = signerStableFileDigest(deltaPath)
	if err != nil {
		return nil, nil, nil, "", err
	}
	return &signerPathReference{name: "base APK", path: basePath, identity: signerFileIdentityOf(baseInfo), hasInfo: true, directory: true},
		&signerPathReference{name: "delta manifest", path: deltaPath, identity: signerFileIdentityOf(deltaInfo), hasInfo: true}, baseRecords, deltaDigest, nil
}

func appendSignerModeReferences(refs []signerPathReference, base, delta *signerPathReference) ([]signerPathReference, error) {
	if base == nil && delta == nil {
		return refs, nil
	}
	if base == nil || delta == nil {
		return nil, fmt.Errorf("%w: incomplete delta paths", ErrInvalidSignerPlan)
	}
	return append(refs, *base, *delta), nil
}

func rejectSignerPathOverlap(refs []signerPathReference) error {
	for index, first := range refs {
		for _, second := range refs[index+1:] {
			if signerPathsOverlap(first.path, second.path) || (first.hasInfo && second.hasInfo && signerSameIdentity(first.identity, second.identity)) {
				return fmt.Errorf("%w: %s and %s paths alias", ErrInvalidSignerPlan, first.name, second.name)
			}
		}
	}
	return nil
}
