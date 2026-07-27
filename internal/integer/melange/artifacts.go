package melange

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

const artifactMarkerName = "verity-spec.sha256"

func ArtifactsExist(paths *Paths, spec Spec, arch Architecture) bool {
	if !arch.valid() {
		return false
	}
	marker, err := regularFile(paths.RepoDir, filepath.Join(string(arch), artifactMarkerName))
	if err != nil {
		return false
	}
	expected, err := artifactFingerprint(paths, spec, arch)
	if err != nil {
		return false
	}
	actual, err := os.ReadFile(marker)
	return err == nil && string(actual) == expected
}

func writeArtifactMarker(paths *Paths, spec Spec, arch Architecture) error {
	fingerprint, err := artifactFingerprint(paths, spec, arch)
	if err != nil {
		return err
	}
	path := artifactMarkerPath(paths, arch)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create artifact marker directory: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace artifact marker: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create artifact marker: %w", err)
	}
	if _, err := file.WriteString(fingerprint); err != nil {
		_ = file.Close()
		return fmt.Errorf("write artifact marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close artifact marker: %w", err)
	}
	return nil
}

type artifactIdentity struct {
	Spec          Spec              `json:"spec"`
	Arch          string            `json:"arch"`
	LockSHA256    string            `json:"lock_sha256"`
	InputSHA256s  map[string]string `json:"input_sha256s"`
	OutputSHA256s map[string]string `json:"output_sha256s"`
}

func artifactFingerprint(paths *Paths, spec Spec, arch Architecture) (string, error) {
	lockData, err := readRegularFile(filepath.Dir(paths.LockFile), filepath.Base(paths.LockFile))
	if err != nil {
		return "", fmt.Errorf("read lock file: %w", err)
	}
	var lock lockFile
	if err := json.Unmarshal(lockData, &lock); err != nil {
		return "", fmt.Errorf("parse lock file: %w", err)
	}
	inputs, err := artifactInputDigests(paths, lock, spec)
	if err != nil {
		return "", err
	}
	outputs, err := artifactOutputDigests(paths, arch)
	if err != nil {
		return "", err
	}
	lockSum := sha256.Sum256(lockData)
	data, err := json.Marshal(artifactIdentity{
		Spec:          spec,
		Arch:          string(arch),
		LockSHA256:    hex.EncodeToString(lockSum[:]),
		InputSHA256s:  inputs,
		OutputSHA256s: outputs,
	})
	if err != nil {
		return "", fmt.Errorf("marshal artifact identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func artifactInputDigests(paths *Paths, lock lockFile, spec Spec) (map[string]string, error) {
	inputs := map[string]string{}
	var err error
	if spec.Upstream != "" {
		err = addLockedInputDigests(inputs, paths, lock, spec.Upstream)
	} else {
		err = addBespokeInputDigests(inputs, paths, spec.Bespoke)
	}
	if err != nil {
		return nil, err
	}
	if err := addPipelineInputDigests(inputs, paths, lock.PipelineFiles); err != nil {
		return nil, err
	}
	if err := addOverrideInputDigest(inputs, paths, spec.EnvFile); err != nil {
		return nil, err
	}
	return inputs, nil
}

func addLockedInputDigests(inputs map[string]string, paths *Paths, lock lockFile, key string) error {
	entry, ok := lock.Packages[key]
	if !ok || entry.File == "" || entry.SHA256 == "" {
		return fmt.Errorf("%w: %q", errMissingLockMetadata, key)
	}
	recipe, err := readVerifiedFile(paths.LockedDir, entry.File, entry.SHA256)
	if err != nil {
		return fmt.Errorf("verify recipe %s: %w", key, err)
	}
	recordDigest(inputs, "recipe/"+entry.File, recipe)
	sidecarName := entry.File[:len(entry.File)-len(filepath.Ext(entry.File))]
	sidecarDir, err := secureOptionalDir(paths.LockedDir, sidecarName)
	if err != nil {
		return fmt.Errorf("verify recipe %s sidecar: %w", key, err)
	}
	actual, err := treeFiles(sidecarDir, filepath.ToSlash(sidecarName))
	if err != nil {
		return fmt.Errorf("list recipe %s sidecar: %w", key, err)
	}
	if err := compareFileSet("recipe "+key+" sidecar", mapKeys(entry.Assets), actual); err != nil {
		return err
	}
	for _, asset := range mapKeys(entry.Assets) {
		data, err := readVerifiedFile(paths.LockedDir, asset, entry.Assets[asset])
		if err != nil {
			return fmt.Errorf("verify recipe asset %s: %w", asset, err)
		}
		recordDigest(inputs, "asset/"+asset, data)
	}
	return nil
}

func addBespokeInputDigests(inputs map[string]string, paths *Paths, sourceFiles []string) error {
	files := slices.Clone(sourceFiles)
	slices.Sort(files)
	for _, file := range files {
		data, err := readRegularFile(paths.BespokeDir, file)
		if err != nil {
			return fmt.Errorf("verify bespoke recipe %s: %w", file, err)
		}
		recordDigest(inputs, "bespoke/"+file, data)
	}
	return nil
}

func addPipelineInputDigests(inputs map[string]string, paths *Paths, expected map[string]string) error {
	actualPipelines, err := treeFiles(paths.PipelinesDir, "")
	if err != nil {
		return fmt.Errorf("list shared pipelines: %w", err)
	}
	if err := compareFileSet("pipeline", mapKeys(expected), actualPipelines); err != nil {
		return err
	}
	for _, file := range mapKeys(expected) {
		data, err := readVerifiedFile(paths.PipelinesDir, file, expected[file])
		if err != nil {
			return fmt.Errorf("verify pipeline %s: %w", file, err)
		}
		recordDigest(inputs, "pipeline/"+file, data)
	}
	return nil
}

func addOverrideInputDigest(inputs map[string]string, paths *Paths, envFile string) error {
	if envFile == "" {
		return nil
	}
	data, err := readRegularFile(paths.OverridesDir, envFile)
	if err != nil {
		return fmt.Errorf("verify environment file %s: %w", envFile, err)
	}
	recordDigest(inputs, "override/"+envFile, data)
	return nil
}

func recordDigest(digests map[string]string, label string, data []byte) {
	sum := sha256.Sum256(data)
	digests[label] = hex.EncodeToString(sum[:])
}

func artifactOutputDigests(paths *Paths, arch Architecture) (map[string]string, error) {
	archDir, err := secureOptionalDir(paths.RepoDir, string(arch))
	if err != nil {
		return nil, fmt.Errorf("verify package output directory: %w", err)
	}
	if archDir == "" {
		return nil, errNoPackageIndex
	}
	files, err := treeFiles(archDir, "")
	if err != nil {
		return nil, fmt.Errorf("list package outputs: %w", err)
	}
	outputs := map[string]string{}
	for _, file := range files {
		if file == artifactMarkerName || file == "melange.rsa.pub" {
			continue
		}
		data, err := readRegularFile(archDir, file)
		if err != nil {
			return nil, fmt.Errorf("verify package output %s: %w", file, err)
		}
		sum := sha256.Sum256(data)
		outputs["package/"+file] = hex.EncodeToString(sum[:])
	}
	if _, ok := outputs["package/APKINDEX.tar.gz"]; !ok {
		return nil, errNoPackageIndex
	}
	keyData, err := readRegularFile(archDir, "melange.rsa.pub")
	if err != nil {
		return nil, fmt.Errorf("verify public key: %w", err)
	}
	keySum := sha256.Sum256(keyData)
	outputs["key/melange.rsa.pub"] = hex.EncodeToString(keySum[:])
	return outputs, nil
}

func artifactMarkerPath(paths *Paths, arch Architecture) string {
	return filepath.Join(paths.RepoDir, string(arch), artifactMarkerName)
}
