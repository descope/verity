//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	kindConfigEnv     = "VERITY_IT_KIND_CONFIG"
	kindCreateWaitEnv = "VERITY_IT_KIND_CREATE_WAIT"
)

func kindConfigPath(repoRoot string) string {
	value := strings.TrimSpace(os.Getenv(kindConfigEnv))
	if value == "" {
		return filepath.Join(repoRoot, "test", "chart-integration", "kind.yaml")
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(repoRoot, value)
}

func kindCreateWait() string {
	value := strings.TrimSpace(os.Getenv(kindCreateWaitEnv))
	if value == "" {
		return "120s"
	}
	return value
}
