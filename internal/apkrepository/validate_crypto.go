package apkrepository

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var trustedPackageCount = regexp.MustCompile(`(^| )[1-9]\d* distinct packages available`)

func verifyRepositoryCrypto(
	ctx context.Context,
	repository string,
	layouts []architectureLayout,
	publicKeys []string,
	runner commandRunner,
) error {
	byArchitecture := make(map[string]architectureLayout, len(layouts))
	for _, layout := range layouts {
		byArchitecture[layout.name] = layout
	}
	for _, required := range []string{"x86_64", "aarch64"} {
		if len(byArchitecture[required].packages) == 0 {
			return fmt.Errorf("%w: %s", errRequiredArchitectureMissing, required)
		}
	}
	for _, layout := range layouts {
		if err := verifyArchitectureCrypto(ctx, repository, &layout, publicKeys, runner); err != nil {
			return err
		}
	}
	return nil
}

func verifyArchitectureCrypto(
	ctx context.Context,
	repository string,
	layout *architectureLayout,
	publicKeys []string,
	runner commandRunner,
) error {
	for _, packagePath := range layout.packages {
		if _, err := runRequired(ctx, runner, &command{
			name: "apk",
			args: []string{"verify", "--keys-dir", repository, packagePath},
		}); err != nil {
			return fmt.Errorf("APK signature verification failed: %s: %w", packagePath, err)
		}
	}
	clientRoot, err := os.MkdirTemp("", "verity-apk-client-")
	if err != nil {
		return fmt.Errorf("create client root: %w", err)
	}
	defer os.RemoveAll(clientRoot)
	trustedRoot := filepath.Join(clientRoot, "root")
	if err := initializeClient(ctx, runner, trustedRoot, layout.name); err != nil {
		return err
	}
	trustedKeys := filepath.Join(trustedRoot, "etc", "apk", "keys")
	trustedRepo := filepath.Join(trustedRoot, "repo", layout.name)
	if err := os.MkdirAll(trustedKeys, 0o755); err != nil {
		return fmt.Errorf("create trusted keys directory: %w", err)
	}
	if err := os.MkdirAll(trustedRepo, 0o755); err != nil {
		return fmt.Errorf("create trusted repository directory: %w", err)
	}
	for _, publicKey := range publicKeys {
		if err := copyFile(publicKey, filepath.Join(trustedKeys, filepath.Base(publicKey))); err != nil {
			return err
		}
	}
	indexPath := filepath.Join(layout.directory, "APKINDEX.tar.gz")
	if err := copyFile(indexPath, filepath.Join(trustedRepo, "APKINDEX.tar.gz")); err != nil {
		return err
	}
	repositoriesFile := filepath.Join(clientRoot, "repositories")
	if err := writeRepositoriesFile(repositoriesFile, trustedRoot); err != nil {
		return err
	}
	result, err := runRequired(ctx, runner, updateCommand(trustedRoot, layout.name, trustedKeys, repositoriesFile))
	if err != nil {
		return fmt.Errorf("fresh-client repository update failed: %s: %w", layout.name, err)
	}
	output := string(append(result.stdout, result.stderr...))
	if containsSignatureFailure(output) || !trustedPackageCount.MatchString(output) {
		return fmt.Errorf("%w: %s: %s", errTrustedUpdateFailed, layout.name, strings.TrimSpace(output))
	}
	return rejectWrongKey(ctx, runner, clientRoot, indexPath, layout)
}

func initializeClient(ctx context.Context, runner commandRunner, root, architecture string) error {
	_, err := runRequired(ctx, runner, &command{
		name: "apk",
		args: []string{"--root", root, "--arch", architecture, "--repositories-file", "/dev/null", "add", "--initdb"},
	})
	if err != nil {
		return fmt.Errorf("initialize apk client for %s: %w", architecture, err)
	}
	return nil
}

func updateCommand(root, architecture, keysDir, repositoriesFile string) *command {
	return &command{
		name: "apk",
		args: []string{
			"--root", root,
			"--arch", architecture,
			"--keys-dir", keysDir,
			"--repositories-file", repositoriesFile,
			"--no-cache", "update",
		},
	}
}

func rejectWrongKey(
	ctx context.Context,
	runner commandRunner,
	clientRoot string,
	indexPath string,
	layout *architectureLayout,
) error {
	wrongKeys := filepath.Join(clientRoot, "wrong-keys")
	if err := os.MkdirAll(wrongKeys, 0o755); err != nil {
		return fmt.Errorf("create wrong keys directory: %w", err)
	}
	keyName := strings.TrimPrefix(layout.signatures[0], ".SIGN.RSA256.")
	if err := writeWrongPublicKey(filepath.Join(wrongKeys, keyName)); err != nil {
		return err
	}
	verifyResult, err := runner.Run(ctx, &command{
		name: "apk",
		args: []string{"verify", "--keys-dir", wrongKeys, layout.packages[0]},
	})
	if err != nil {
		return err
	}
	if verifyResult.exitCode == 0 {
		return fmt.Errorf("%w: %s", errWrongKeyVerifiedPackage, layout.packages[0])
	}
	wrongRoot := filepath.Join(clientRoot, "wrong-root")
	if err := initializeClient(ctx, runner, wrongRoot, layout.name); err != nil {
		return err
	}
	wrongRootKeys := filepath.Join(wrongRoot, "etc", "apk", "keys")
	wrongRepo := filepath.Join(wrongRoot, "repo", layout.name)
	if err := os.MkdirAll(wrongRootKeys, 0o755); err != nil {
		return fmt.Errorf("create wrong root keys: %w", err)
	}
	if err := os.MkdirAll(wrongRepo, 0o755); err != nil {
		return fmt.Errorf("create wrong repository directory: %w", err)
	}
	if err := copyFile(filepath.Join(wrongKeys, keyName), filepath.Join(wrongRootKeys, keyName)); err != nil {
		return err
	}
	if err := copyFile(indexPath, filepath.Join(wrongRepo, "APKINDEX.tar.gz")); err != nil {
		return err
	}
	wrongRepositoriesFile := filepath.Join(clientRoot, "wrong-repositories")
	if err := writeRepositoriesFile(wrongRepositoriesFile, wrongRoot); err != nil {
		return err
	}
	result, err := runner.Run(ctx, updateCommand(wrongRoot, layout.name, wrongRootKeys, wrongRepositoriesFile))
	if err != nil {
		return err
	}
	output := string(append(result.stdout, result.stderr...))
	if !containsSignatureFailure(output) {
		return fmt.Errorf("%w: %s", errWrongKeyVerifiedIndex, layout.name)
	}
	return nil
}

func writeRepositoriesFile(path, root string) error {
	repositoryURL := (&url.URL{Scheme: "file", Path: filepath.Join(root, "repo")}).String()
	if err := os.WriteFile(path, []byte(repositoryURL+"\n"), 0o644); err != nil {
		return fmt.Errorf("write repositories file: %w", err)
	}
	return nil
}

func writeWrongPublicKey(path string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate negative-test RSA key: %w", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("encode negative-test RSA key: %w", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write negative-test RSA key: %w", err)
	}
	return nil
}

func containsSignatureFailure(output string) bool {
	return strings.Contains(output, "BAD signature") || strings.Contains(output, "UNTRUSTED signature")
}
