package repositoryops

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxSBOMBytes = 16 << 20

var (
	ErrNativeVerification = errors.New("native image verification failed")
	nativeVersionPattern  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,127}$`)
	containerIDPattern    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
)

type SealedSecretsImageInput struct {
	Image       string
	Version     string
	FullVersion string
	TempDir     string
}

type SealedSecretsImageRequest struct {
	image       string
	version     string
	fullVersion string
	tempDir     string
}

func NewSealedSecretsImageRequest(input SealedSecretsImageInput) (SealedSecretsImageRequest, error) {
	image, err := validatedImageReference(input.Image)
	if err != nil {
		return SealedSecretsImageRequest{}, err
	}
	version := strings.TrimSpace(input.Version)
	fullVersion := strings.TrimSpace(input.FullVersion)
	if !nativeVersionPattern.MatchString(version) || !nativeVersionPattern.MatchString(fullVersion) {
		return SealedSecretsImageRequest{}, fmt.Errorf("%w: invalid version", ErrNativeVerification)
	}
	tempDir, err := validatedPath("temporary directory", input.TempDir)
	if err != nil {
		return SealedSecretsImageRequest{}, err
	}
	info, err := os.Stat(tempDir)
	if err != nil || !info.IsDir() {
		return SealedSecretsImageRequest{}, fmt.Errorf("%w: temporary directory %q is unavailable", ErrNativeVerification, tempDir)
	}
	return SealedSecretsImageRequest{image: image, version: version, fullVersion: fullVersion, tempDir: tempDir}, nil
}

func (s NativeService) VerifySealedSecretsImage(ctx context.Context, request SealedSecretsImageRequest) (err error) {
	commands := s.Commands
	if commands == nil {
		commands = ExecCommandRunner{}
	}
	version, err := runRequiredCommand(ctx, commands, &Command{
		Name: "docker", Args: []string{"run", "--rm", "--entrypoint", "/usr/bin/controller", request.image, "--version"},
	})
	if err != nil {
		return fmt.Errorf("controller version: %w", err)
	}
	wantVersion := "controller version: v" + request.version
	if strings.TrimSpace(string(version.Stdout)) != wantVersion {
		return fmt.Errorf("%w: controller version output %q, want %q", ErrNativeVerification, strings.TrimSpace(string(version.Stdout)), wantVersion)
	}
	for _, executable := range []string{"/usr/bin/controller", "/usr/bin/kubeseal"} {
		if _, err := runRequiredCommand(ctx, commands, &Command{
			Name: "docker", Args: []string{"run", "--rm", "--entrypoint", executable, request.image, "--help"},
		}); err != nil {
			return fmt.Errorf("%s help: %w", filepath.Base(executable), err)
		}
	}
	created, err := runRequiredCommand(ctx, commands, &Command{Name: "docker", Args: []string{"create", request.image}})
	if err != nil {
		return fmt.Errorf("create verification container: %w", err)
	}
	container := strings.TrimSpace(string(created.Stdout))
	if !containerIDPattern.MatchString(container) {
		return fmt.Errorf("%w: malformed container identifier %q", ErrNativeVerification, container)
	}
	defer func() {
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if _, cleanupErr := runRequiredCommand(cleanupContext, commands, &Command{
			Name: "docker", Args: []string{"rm", "--force", container},
		}); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove verification container: %w", cleanupErr))
		}
	}()
	verifier := &sealedSecretsVerifier{commands: commands, request: request, container: container}
	if err := verifier.verifyCertificate(ctx); err != nil {
		return err
	}
	return verifier.verifySBOM(ctx)
}

type sealedSecretsVerifier struct {
	commands  CommandRunner
	request   SealedSecretsImageRequest
	container string
}

func (v *sealedSecretsVerifier) verifyCertificate(ctx context.Context) (err error) {
	tarFile, err := os.CreateTemp(v.request.tempDir, "sealed-secrets-rootfs-*.tar")
	if err != nil {
		return fmt.Errorf("create rootfs archive: %w", err)
	}
	path := tarFile.Name()
	defer func() {
		if closeErr := tarFile.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rootfs archive: %w", closeErr))
		}
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, fmt.Errorf("remove rootfs archive: %w", removeErr))
		}
	}()
	if _, err := runRequiredCommand(ctx, v.commands, &Command{Name: "docker", Args: []string{"export", v.container}, Stdout: tarFile}); err != nil {
		return fmt.Errorf("export verification container: %w", err)
	}
	if _, err := tarFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind rootfs archive: %w", err)
	}
	reader := tar.NewReader(tarFile)
	for {
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			return fmt.Errorf("%w: CA certificate bundle is missing", ErrNativeVerification)
		}
		if readErr != nil {
			return fmt.Errorf("%w: read rootfs archive: %w", ErrNativeVerification, readErr)
		}
		clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(header.Name)), "./")
		if clean == "etc/ssl/cert.pem" || clean == "etc/ssl/certs/ca-certificates.crt" {
			return nil
		}
	}
}

func (v *sealedSecretsVerifier) verifySBOM(ctx context.Context) (err error) {
	sbomPath := filepath.Join(v.request.tempDir, "sealed-secrets-"+v.request.fullVersion+".spdx.json")
	defer func() {
		if removeErr := os.Remove(sbomPath); removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, fmt.Errorf("remove sealed-secrets SBOM: %w", removeErr))
		}
	}()
	containerPath := v.container + ":/var/lib/db/sbom/sealed-secrets-0-" + v.request.fullVersion + ".spdx.json"
	if _, err := runRequiredCommand(ctx, v.commands, &Command{Name: "docker", Args: []string{"cp", containerPath, sbomPath}}); err != nil {
		return fmt.Errorf("copy sealed-secrets SBOM: %w", err)
	}
	info, err := os.Lstat(sbomPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSBOMBytes {
		return fmt.Errorf("%w: SBOM is missing, non-regular, or oversized", ErrNativeVerification)
	}
	data, err := os.ReadFile(sbomPath)
	if err != nil {
		return fmt.Errorf("read sealed-secrets SBOM: %w", err)
	}
	var document spdxDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("%w: parse SBOM: %w", ErrNativeVerification, err)
	}
	if document.SPDXVersion != "SPDX-2.3" {
		return fmt.Errorf("%w: SPDX version is %q", ErrNativeVerification, document.SPDXVersion)
	}
	for _, pkg := range document.Packages {
		if pkg.Name == "sealed-secrets-0" && pkg.VersionInfo == v.request.fullVersion && pkg.LicenseDeclared == "Apache-2.0" {
			return nil
		}
	}
	return fmt.Errorf("%w: sealed-secrets package metadata is missing", ErrNativeVerification)
}

type spdxDocument struct {
	SPDXVersion string        `json:"spdxVersion"`
	Packages    []spdxPackage `json:"packages"`
}

type spdxPackage struct {
	Name            string `json:"name"`
	VersionInfo     string `json:"versionInfo"`
	LicenseDeclared string `json:"licenseDeclared"`
}
