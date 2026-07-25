package buildmetadata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var (
	// ErrNoBuildInputs reports a root without declared production inputs.
	ErrNoBuildInputs     = errors.New("no build inputs")
	errInvalidBuildInput = errors.New("invalid build input")
)

// BuildConfig contains target and compiler inputs that affect a build.
type BuildConfig struct {
	GOOS       string
	GOARCH     string
	CGOEnabled string
	Flags      []string
}

// BuildKeyOptions identifies the source tree and target configuration to hash.
type BuildKeyOptions struct {
	Root   string
	Config BuildConfig
}

// CanonicalBuildConfig returns the current target's reproducible build inputs.
func CanonicalBuildConfig() BuildConfig {
	details := runtimeSettings()
	cgo := os.Getenv("CGO_ENABLED")
	if cgo == "" {
		cgo = UnknownValue
	}
	return BuildConfig{
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		CGOEnabled: cgo,
		Flags:      append([]string(nil), details.BuildFlags...),
	}
}

// ComputeBuildKey returns a deterministic SHA-256 identity for build inputs.
//
//nolint:gocritic // Keep the value options API aligned with the build seam.
func ComputeBuildKey(ctx context.Context, options BuildKeyOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("compute build key: %w", err)
	}
	info, err := os.Stat(options.Root)
	if err != nil {
		return "", fmt.Errorf("inspect build root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("compute build key: %w", ErrNoBuildInputs)
	}

	files, err := collectBuildInputs(ctx, options.Root)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("compute build key: %w", ErrNoBuildInputs)
	}
	return hashBuildInputs(ctx, options.Root, files, options.Config)
}

func collectBuildInputs(ctx context.Context, root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative != "." && excludedDirectory(relative) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errInvalidBuildInput
		}
		if isBuildInput(relative) {
			files = append(files, relative)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk build inputs: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func hashBuildInputs(ctx context.Context, root string, files []string, config BuildConfig) (string, error) {
	config.Flags = append([]string(nil), config.Flags...)
	sort.Strings(config.Flags)
	configData, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal build configuration: %w", err)
	}
	hasher := sha256.New()
	hasher.Write([]byte("verity-build-key-v1\x00"))
	hasher.Write(configData)
	hasher.Write([]byte{0})
	for _, relative := range files {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("compute build key: %w", err)
		}
		data, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			return "", fmt.Errorf("read build input: %w", err)
		}
		hasher.Write([]byte(relative))
		hasher.Write([]byte{0})
		hasher.Write(data)
		hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func isBuildInput(path string) bool {
	if excludedDirectory(path) {
		return false
	}
	base := filepath.Base(path)
	switch base {
	case "go.mod", "go.sum", "go.work", "go.work.sum", "mise.toml":
		return true
	default:
		return strings.HasSuffix(base, ".go") && !strings.HasSuffix(base, "_test.go")
	}
}

func excludedDirectory(path string) bool {
	for segment := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		switch segment {
		case ".git", "vendor", "node_modules", "testdata", "testonly":
			return true
		}
	}
	return false
}
