package apkrepository

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type DeltaOptions struct {
	BaseDir       string
	PackageDir    string
	ManifestPath  string
	OutputDir     string
	KeyName       string
	PrivateKeyPEM []byte
	Stdout        io.Writer
	Stderr        io.Writer
	runner        commandRunner
}

type deltaConfig struct {
	baseDir       string
	packageDir    string
	manifestPath  string
	outputDir     string
	keyName       string
	privateKeyPEM []byte
	stdout        io.Writer
	stderr        io.Writer
	runner        commandRunner
}

func ApplyDelta(ctx context.Context, options *DeltaOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply APK delta: %w", err)
	}
	config, err := parseDeltaOptions(options)
	if err != nil {
		return err
	}
	if err := Validate(ctx, &ValidateOptions{RepositoryDir: config.baseDir, RequireSignature: true}); err != nil {
		return fmt.Errorf("validate APK delta base: %w", err)
	}
	manifest, err := readDeltaManifest(config.manifestPath, config.packageDir)
	if err != nil {
		return err
	}
	base, err := loadDeltaBase(config, &manifest)
	if err != nil {
		return err
	}
	plan, err := buildDeltaPlan(config, &manifest, base)
	if err != nil {
		return err
	}
	stage, err := prepareStagedOutput(config.outputDir)
	if err != nil {
		return err
	}
	defer stage.cleanup()
	if err := copySelectedRepository(config.baseDir, stage.path); err != nil {
		return err
	}
	resultWriter := config.stdout
	var result bytes.Buffer
	config.stdout = &result
	privateKey, cleanup, err := prepareDeltaSigningKey(config, base, plan)
	if err != nil {
		return err
	}
	defer cleanup()
	execution := deltaExecution{
		ctx: ctx, config: config, plan: plan, stage: stage.path, privateKey: privateKey,
	}
	if err := execution.apply(); err != nil {
		return err
	}
	if err := stage.commit(); err != nil {
		return err
	}
	if _, err := io.Copy(resultWriter, &result); err != nil {
		return fmt.Errorf("write delta result: %w", err)
	}
	_, err = fmt.Fprintf(resultWriter, "Applied APK delta: %d changed, %d byte-preserved\n", plan.changed, plan.unchanged)
	return err
}

func parseDeltaOptions(options *DeltaOptions) (*deltaConfig, error) {
	if info, err := os.Stat(options.BaseDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, options.BaseDir)
	}
	if options.ManifestPath == "" {
		return nil, fmt.Errorf("%w: manifest path is required", errInvalidDeltaManifest)
	}
	outputDir := options.OutputDir
	if outputDir == "" {
		outputDir = "site/dist/apk"
	}
	if err := validateOutputDirectory(outputDir); err != nil {
		return nil, err
	}
	keyName := options.KeyName
	if keyName == "" {
		keyName = "verity.rsa"
	}
	if !safeKeyName.MatchString(keyName) || strings.Contains(keyName, "..") {
		return nil, fmt.Errorf("%w: %s", errUnsafeKeyName, keyName)
	}
	if !strings.HasSuffix(keyName, ".rsa") {
		return nil, fmt.Errorf("%w: %s", errRSAKeyNameRequired, keyName)
	}
	packageDir := options.PackageDir
	if packageDir == "" {
		packageDir = "."
	}
	runner := options.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &deltaConfig{
		baseDir: options.BaseDir, packageDir: packageDir, manifestPath: options.ManifestPath,
		outputDir: outputDir, keyName: keyName, privateKeyPEM: options.PrivateKeyPEM,
		stdout: writerOrDiscard(options.Stdout), stderr: writerOrDiscard(options.Stderr), runner: runner,
	}, nil
}

func prepareDeltaSigningKey(config *deltaConfig, base *deltaBase, plan *deltaPlan) (privateKey string, cleanup func(), err error) {
	if len(plan.affectedArches) == 0 {
		return "", func() {}, nil
	}
	if len(config.privateKeyPEM) == 0 {
		return "", func() {}, errSigningKeyRequired
	}
	temporaryDir, err := os.MkdirTemp("", "verity-apk-delta-key-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create delta key directory: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(temporaryDir) }
	privateKey = filepath.Join(temporaryDir, config.keyName)
	if err := prepareSigningKey(config.privateKeyPEM, base.keyPath, privateKey); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return privateKey, cleanup, nil
}
