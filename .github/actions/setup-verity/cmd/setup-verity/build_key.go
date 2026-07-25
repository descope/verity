package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/buildmetadata"
)

type productionBuildManifest struct {
	Toolchain   string                    `json:"toolchain"`
	Config      buildmetadata.BuildConfig `json:"config"`
	Environment []string                  `json:"environment"`
}

type productionBuildHashOptions struct {
	Root      string
	Files     []string
	Config    buildmetadata.BuildConfig
	Toolchain string
}

func computeProductionBuildKey(ctx context.Context, root string, config buildmetadata.BuildConfig) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve build root: %w", err)
	}
	files, err := productionDependencyFiles(ctx, root)
	if err != nil {
		return "", err
	}
	toolchain, err := goCommandOutput(ctx, root, "env", "GOVERSION")
	if err != nil {
		return "", fmt.Errorf("read Go toolchain version: %w", err)
	}
	toolchain = strings.TrimSpace(toolchain)
	if toolchain == "" {
		return "", untrusted("empty Go toolchain version")
	}
	return hashProductionBuildKey(ctx, &productionBuildHashOptions{
		Root: root, Files: files, Config: config, Toolchain: toolchain,
	})
}

func hashProductionBuildKey(ctx context.Context, options *productionBuildHashOptions) (string, error) {
	options.Config.Flags = append([]string(nil), options.Config.Flags...)
	manifest, err := json.Marshal(productionBuildManifest{
		Toolchain: options.Toolchain, Config: options.Config,
		Environment: append([]string(nil), canonicalBuildEnvironment...),
	})
	if err != nil {
		return "", fmt.Errorf("marshal production build manifest: %w", err)
	}
	files := append([]string(nil), options.Files...)
	sort.Strings(files)
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("verity-production-build-key-v3\x00"))
	_, _ = hasher.Write(manifest)
	_, _ = hasher.Write([]byte{0})
	for index, relative := range files {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("hash production build inputs: %w", err)
		}
		if index > 0 && relative == files[index-1] {
			return "", untrusted("duplicate production build input")
		}
		relative, err = canonicalProductionRelativePath(relative)
		if err != nil {
			return "", err
		}
		path := filepath.Join(options.Root, filepath.FromSlash(relative))
		data, err := readStableProductionFile(path)
		if err != nil {
			return "", fmt.Errorf("read production build input %q: %w", relative, err)
		}
		_, _ = hasher.Write([]byte(relative))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write(data)
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func goCommandOutput(ctx context.Context, root string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "go", arguments...)
	command.Dir = root
	command.Env = buildEnvironment(os.Environ())
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("go %s: %w: %s", arguments[0], err, strings.TrimSpace(stderr.String()))
	}
	return string(output), nil
}
