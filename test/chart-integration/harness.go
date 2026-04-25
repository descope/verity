//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const clusterName = "verity-it"

type Harness struct {
	KubeconfigPath string
	RepoRoot       string
	clusterCreated bool
	t              testLogger
}

type testLogger interface {
	Logf(format string, args ...any)
	Helper()
}

func NewHarness(t testLogger, repoRoot string) *Harness {
	return &Harness{RepoRoot: repoRoot, t: t}
}

func (h *Harness) Setup(ctx context.Context) error {
	h.t.Helper()
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"create kind cluster", h.createCluster},
		{"export kubeconfig", h.exportKubeconfig},
	}
	for _, s := range steps {
		h.t.Logf("[harness] %s", s.name)
		if err := s.fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return nil
}

func (h *Harness) Teardown(ctx context.Context) {
	h.t.Helper()
	if h.clusterCreated {
		h.t.Logf("[harness] deleting kind cluster %s", clusterName)
		if err := runCmd(ctx, h.t, "", nil, "kind", "delete", "cluster", "--name", clusterName); err != nil {
			h.t.Logf("[harness] kind delete failed (continuing): %v", err)
		}
	} else {
		h.t.Logf("[harness] preserving pre-existing cluster %s (not created by this run)", clusterName)
	}
	if h.KubeconfigPath != "" {
		if err := os.Remove(h.KubeconfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			h.t.Logf("[harness] kubeconfig cleanup failed: %v", err)
		}
	}
}

func (h *Harness) createCluster(ctx context.Context) error {
	out, listErr := exec.CommandContext(ctx, "kind", "get", "clusters").CombinedOutput()
	if listErr == nil {
		for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			if strings.TrimSpace(line) == clusterName {
				h.t.Logf("[harness] reusing pre-existing cluster %s (will not delete on teardown)", clusterName)
				h.clusterCreated = false
				return nil
			}
		}
	}
	cfg := filepath.Join(h.RepoRoot, "test", "chart-integration", "kind.yaml")
	if err := runCmd(ctx, h.t, "", nil,
		"kind", "create", "cluster",
		"--name", clusterName,
		"--config", cfg,
		"--wait", "120s",
	); err != nil {
		return err
	}
	h.clusterCreated = true
	return nil
}

func (h *Harness) exportKubeconfig(ctx context.Context) error {
	tmp, err := os.CreateTemp("", "verity-it-kubeconfig-*.yaml")
	if err != nil {
		return err
	}
	if cerr := tmp.Close(); cerr != nil {
		return cerr
	}
	h.KubeconfigPath = tmp.Name()
	out, err := exec.CommandContext(ctx, "kind", "get", "kubeconfig", "--name", clusterName).CombinedOutput()
	if err != nil {
		stderr := strings.TrimSpace(string(out))
		if stderr != "" {
			return fmt.Errorf("kind get kubeconfig: %s: %w", stderr, err)
		}
		return fmt.Errorf("kind get kubeconfig: %w", err)
	}
	if err := os.WriteFile(tmp.Name(), out, 0o600); err != nil {
		return err
	}
	return nil
}

func runCmd(ctx context.Context, t testLogger, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("exec %s %s: %s: %w", name, strings.Join(args, " "), string(out), err)
	}
	if t != nil && len(out) > 0 {
		t.Logf("[exec %s] %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCmdOutput(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec %s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}
