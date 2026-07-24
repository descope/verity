package apkrepository

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ValidateOptions struct {
	RepositoryDir    string
	RequireSignature bool
	VerifyCrypto     bool
	Stdout           io.Writer
	Stderr           io.Writer
	runner           commandRunner
}

func Validate(ctx context.Context, options *ValidateOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	info, err := os.Stat(options.RepositoryDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w: %s", errRepositoryNotFound, options.RepositoryDir)
	}
	packages, err := validatePackageDepth(options.RepositoryDir)
	if err != nil {
		return err
	}
	stdout := writerOrDiscard(options.Stdout)
	if len(packages) == 0 {
		if _, err := os.Stat(filepath.Join(options.RepositoryDir, ".no-apks-found")); err == nil {
			_, writeErr := fmt.Fprintln(stdout, "No APK files present; guarded empty repository marker found")
			return writeErr
		}
		return errEmptyMarkerMissing
	}
	requireSignature := options.RequireSignature || options.VerifyCrypto
	publicKeys, err := repositoryPublicKeys(options.RepositoryDir)
	if err != nil {
		return err
	}
	if requireSignature && len(publicKeys) == 0 {
		return errSignaturePublicKeyMissing
	}
	architectures, err := validateArchitectureLayouts(options.RepositoryDir, requireSignature)
	if err != nil {
		return err
	}
	if options.VerifyCrypto {
		runner := options.runner
		if runner == nil {
			runner = execCommandRunner{}
		}
		if err := verifyRepositoryCrypto(ctx, options.RepositoryDir, architectures, publicKeys, runner); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(stdout, "APK repository layout valid: %s (%d packages)\n", options.RepositoryDir, len(packages))
	return err
}

func validatePackageDepth(repository string) ([]string, error) {
	packages := make([]string, 0)
	err := filepath.WalkDir(repository, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".apk") {
			return nil
		}
		relative, err := filepath.Rel(repository, path)
		if err != nil {
			return err
		}
		depth := len(strings.Split(filepath.ToSlash(relative), "/"))
		switch {
		case depth == 1:
			return fmt.Errorf("%w: %s", errRootPackage, path)
		case depth > 2:
			return fmt.Errorf("%w: %s", errNestedPackage, path)
		default:
			packages = append(packages, path)
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(packages)
	return packages, nil
}

type architectureLayout struct {
	name       string
	directory  string
	packages   []string
	signatures []string
}

func validateArchitectureLayouts(repository string, requireSignature bool) ([]architectureLayout, error) {
	entries, err := os.ReadDir(repository)
	if err != nil {
		return nil, fmt.Errorf("read repository: %w", err)
	}
	layouts := make([]architectureLayout, 0)
	var validationErrors []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(repository, entry.Name())
		packages, globErr := filepath.Glob(filepath.Join(directory, "*.apk"))
		if globErr != nil || len(packages) == 0 {
			continue
		}
		if !isSupportedArchitecture(entry.Name()) {
			validationErrors = append(validationErrors, fmt.Errorf("%w: %s", errUnsupportedArchitecture, entry.Name()))
			continue
		}
		indexPath := filepath.Join(directory, "APKINDEX.tar.gz")
		signatures, indexErr := readIndexSignatures(indexPath)
		if indexErr != nil {
			validationErrors = append(validationErrors, indexErr)
			continue
		}
		if requireSignature && len(signatures) == 0 {
			validationErrors = append(validationErrors, fmt.Errorf("%w in %s", errSignatureMissing, indexPath))
			continue
		}
		for _, signature := range signatures {
			keyName := strings.TrimPrefix(signature, ".SIGN.RSA256.")
			if _, err := os.Stat(filepath.Join(repository, keyName)); err != nil {
				validationErrors = append(validationErrors, fmt.Errorf("%w: %s in %s: %s", errSignatureKeyMissing, signature, indexPath, keyName))
			}
		}
		sort.Strings(packages)
		layouts = append(layouts, architectureLayout{name: entry.Name(), directory: directory, packages: packages, signatures: signatures})
	}
	if len(validationErrors) > 0 {
		return nil, errors.Join(validationErrors...)
	}
	return layouts, nil
}

func readIndexSignatures(indexPath string) ([]string, error) {
	file, err := os.Open(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w for %s", errIndexMissing, filepath.Base(filepath.Dir(indexPath)))
		}
		return nil, fmt.Errorf("open index %q: %w", indexPath, err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errInvalidIndex, indexPath)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var signatures []string
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %s", errInvalidIndex, indexPath)
		}
		name := filepath.Base(header.Name)
		if strings.HasPrefix(name, ".SIGN.RSA256.") && strings.HasSuffix(name, ".rsa.pub") {
			signatures = append(signatures, name)
		}
	}
	sort.Strings(signatures)
	return signatures, nil
}

func repositoryPublicKeys(repository string) ([]string, error) {
	keys, err := filepath.Glob(filepath.Join(repository, "*.rsa.pub"))
	if err != nil {
		return nil, fmt.Errorf("list repository public keys: %w", err)
	}
	sort.Strings(keys)
	return keys, nil
}
