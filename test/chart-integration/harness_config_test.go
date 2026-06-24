//go:build integration

package integration

import (
	"path/filepath"
	"testing"
)

func TestKindConfigPathUsesDefault_whenEnvUnset(t *testing.T) {
	// Given
	repoRoot := t.TempDir()

	// When
	got := kindConfigPath(repoRoot)

	// Then
	want := filepath.Join(repoRoot, "test", "chart-integration", "kind.yaml")
	if got != want {
		t.Fatalf("kindConfigPath() = %q, want %q", got, want)
	}
}

func TestKindConfigPathUsesOverride_whenEnvSet(t *testing.T) {
	// Given
	repoRoot := t.TempDir()
	t.Setenv(kindConfigEnv, "test/chart-integration/kind-cilium.yaml")

	// When
	got := kindConfigPath(repoRoot)

	// Then
	want := filepath.Join(repoRoot, "test", "chart-integration", "kind-cilium.yaml")
	if got != want {
		t.Fatalf("kindConfigPath() = %q, want %q", got, want)
	}
}

func TestKindCreateWaitUsesDefault_whenEnvUnset(t *testing.T) {
	// When
	got := kindCreateWait()

	// Then
	if got != "120s" {
		t.Fatalf("kindCreateWait() = %q, want 120s", got)
	}
}

func TestKindCreateWaitUsesOverride_whenEnvSet(t *testing.T) {
	// Given
	t.Setenv(kindCreateWaitEnv, "0s")

	// When
	got := kindCreateWait()

	// Then
	if got != "0s" {
		t.Fatalf("kindCreateWait() = %q, want 0s", got)
	}
}
