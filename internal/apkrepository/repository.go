package apkrepository

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const repositoryFormatVersion = "1"

var (
	errOptionsRequired                = errors.New("APK repository options are required")
	errUnsafeOutputDirectory          = errors.New("unsafe output directory")
	errUnsafeKeyName                  = errors.New("unsafe key name")
	errRSAKeyNameRequired             = errors.New("key name must end with .rsa")
	errUnsupportedPackageArchitecture = errors.New("could not determine APK architecture")
	errDuplicateDestination           = errors.New("duplicate APK destination")
	errCommandFailed                  = errors.New("command failed")
	errInvalidBatchID                 = errors.New("batch ID must have the form RUN_ID-RUN_ATTEMPT")
	errRepositoryEnvironmentRequired  = errors.New("GITHUB_REPOSITORY is required")
	errNoApprovedArtifacts            = errors.New("trusted Integer run did not publish approved APK artifacts")
	errMissingArtifactTar             = errors.New("pages artifact is missing artifact.tar")
	errUnsafeArtifactPath             = errors.New("unsafe Pages artifact path")
	errUnsupportedArtifactEntry       = errors.New("unsupported Pages artifact entry type")
	errCandidateNotFound              = errors.New("candidate APK repository not found")
	errNoPublicKey                    = errors.New("selected APK repository has no public key")
	errPrivateKeyMismatch             = errors.New("APK_REPOSITORY_PRIVATE_KEY does not match committed public key")
	errRepositoryNotFound             = errors.New("repository directory not found")
	errEmptyMarkerMissing             = errors.New("no APK files found and .no-apks-found marker is missing")
	errSignaturePublicKeyMissing      = errors.New("signature required but no public key (*.rsa.pub) found at repository root")
	errRootPackage                    = errors.New("APK files must live under architecture directories, not repository root")
	errNestedPackage                  = errors.New("APK files must live directly under architecture directories")
	errUnsupportedArchitecture        = errors.New("unsupported architecture directory containing APKs")
	errSignatureMissing               = errors.New("missing RSA256 signature entry")
	errSignatureKeyMissing            = errors.New("signature has no matching root public key")
	errIndexMissing                   = errors.New("missing APKINDEX.tar.gz")
	errInvalidIndex                   = errors.New("invalid gzip tar index")
	errRequiredArchitectureMissing    = errors.New("required architecture has no APK packages")
	errTrustedUpdateFailed            = errors.New("fresh-client repository update did not load trusted packages")
	errWrongKeyVerifiedPackage        = errors.New("wrong key unexpectedly verified an APK")
	errWrongKeyVerifiedIndex          = errors.New("wrong key unexpectedly verified repository index")
	supportedArches                   = []string{"x86_64", "aarch64", "armv7", "armhf", "ppc64le", "s390x", "riscv64"}
)

func isSupportedArchitecture(architecture string) bool {
	return slices.Contains(supportedArches, architecture)
}

func validateOutputDirectory(path string) error {
	if path == "" || path == "/" || path == "." {
		return fmt.Errorf("%w: %s", errUnsafeOutputDirectory, path)
	}
	segments := strings.FieldsFunc(filepath.ToSlash(path), func(r rune) bool { return r == '/' })
	if slices.Contains(segments, "..") {
		return fmt.Errorf("%w: %s", errUnsafeOutputDirectory, path)
	}
	return nil
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

func copyFile(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat %q: %w", source, err)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %q: %w", source, err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create destination directory for %q: %w", destination, err)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create %q: %w", destination, err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("copy %q to %q: %w", source, destination, err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close %q: %w", destination, err)
	}
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %q: %w", path, err)
	}
	return nil
}
