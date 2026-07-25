package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type activationOptions struct {
	ArtifactDirectory string
	Destination       string
	Identity          artifactIdentity
}

func activateArtifact(options activationOptions) (err error) {
	verified, err := verifyArtifactDirectory(options.ArtifactDirectory, options.Identity)
	if err != nil {
		return err
	}
	parent, err := os.Lstat(filepath.Dir(options.Destination))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || filepath.Base(options.Destination) != binaryName {
		return untrusted("activation destination")
	}

	source, err := os.Open(verified.BinaryPath)
	if err != nil {
		return fmt.Errorf("open verified binary: %w", err)
	}
	defer func() { err = errorsJoin(err, source.Close()) }()
	temporary, err := os.CreateTemp(filepath.Dir(options.Destination), ".verity-verified-*")
	if err != nil {
		return fmt.Errorf("create activation file: %w", err)
	}
	temporaryPath := temporary.Name()
	temporaryClosed := false
	defer os.Remove(temporaryPath)
	defer func() {
		if !temporaryClosed {
			err = errorsJoin(err, temporary.Close())
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temporary, hasher), source); err != nil {
		return fmt.Errorf("copy verified binary: %w", err)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != verified.BinaryDigest {
		return untrusted("activation copy digest")
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync verified binary: %w", err)
	}
	if err := temporary.Chmod(0o755); err != nil {
		return fmt.Errorf("chmod verified binary: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close verified binary: %w", err)
	}
	temporaryClosed = true
	if err := os.Rename(temporaryPath, options.Destination); err != nil {
		return fmt.Errorf("install verified binary: %w", err)
	}
	return nil
}
