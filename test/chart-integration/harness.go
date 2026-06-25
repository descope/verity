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

const nfsSubdirChartName = "nfs-subdir-external-provisioner"

type Harness struct {
	KubeconfigPath string
	RepoRoot       string
	clusterCreated bool
	nfsInstalled   bool
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
	if needsNFSServer(os.Getenv("VERITY_CHART")) {
		steps = append(steps, struct {
			name string
			fn   func(context.Context) error
		}{"ensure in-cluster NFS server", h.ensureNFSServer})
	}
	for _, s := range steps {
		h.t.Logf("[harness] %s", s.name)
		if err := s.fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return nil
}

func needsNFSServer(chartFilter string) bool {
	filter := strings.TrimSpace(chartFilter)
	return filter == "" || filter == nfsSubdirChartName
}

func (h *Harness) ensureNFSServer(ctx context.Context) error {
	if err := h.configureKindNFSClient(ctx); err != nil {
		return err
	}
	manifest := filepath.Join(h.RepoRoot, "test", "chart-integration", "nfs-server.yaml")
	if err := runCmd(ctx, h.t, "", nil,
		"kubectl", "--kubeconfig", h.KubeconfigPath,
		"apply", "-f", manifest,
	); err != nil {
		return err
	}
	if err := runCmd(ctx, h.t, "", nil,
		"kubectl", "--kubeconfig", h.KubeconfigPath,
		"-n", "verity-it-infra",
		"rollout", "status", "deployment/nfs-server", "--timeout=120s",
	); err != nil {
		return err
	}
	h.nfsInstalled = true
	return nil
}

func (h *Harness) configureKindNFSClient(ctx context.Context) error {
	// nfs-ganesha in the smoke fixture reliably serves NFSv4.0/4.1 in kind,
	// while mount.nfs defaults to trying v4.2 first on the current kind node
	// image. Kubelet's in-tree NFS volume plugin does not expose per-volume
	// mountOptions, so set the node-wide default before the chart pod mounts.
	return runCmd(ctx, h.t, "", nil,
		"docker", "exec", clusterName+"-control-plane", "sh", "-c",
		"mkdir -p /etc/nfsmount.conf.d && printf '[ NFSMount_Global_Options ]\\nDefaultvers=4.1\\n' > /etc/nfsmount.conf.d/verity-nfs.conf",
	)
}

func (h *Harness) Teardown(ctx context.Context) {
	h.t.Helper()
	h.deleteNFSServer(ctx)
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

func (h *Harness) deleteNFSServer(ctx context.Context) {
	if !h.nfsInstalled || h.KubeconfigPath == "" {
		return
	}
	manifest := filepath.Join(h.RepoRoot, "test", "chart-integration", "nfs-server.yaml")
	if err := runCmd(ctx, h.t, "", nil,
		"kubectl", "--kubeconfig", h.KubeconfigPath,
		"delete", "-f", manifest, "--ignore-not-found=true", "--wait=true", "--timeout=120s",
	); err != nil {
		h.t.Logf("[harness] NFS server cleanup failed (continuing): %v", err)
	}
}

func (h *Harness) createCluster(ctx context.Context) error {
	out, listErr := exec.CommandContext(ctx, "kind", "get", "clusters").CombinedOutput()
	if listErr != nil {
		return fmt.Errorf("kind get clusters: %s: %w", strings.TrimSpace(string(out)), listErr)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == clusterName {
			h.t.Logf("[harness] reusing pre-existing cluster %s (will not delete on teardown)", clusterName)
			h.clusterCreated = false
			return nil
		}
	}
	h.cleanupStaleKindNode(ctx)
	args := []string{
		"create", "cluster",
		"--name", clusterName,
		"--config", kindConfigPath(h.RepoRoot),
	}
	if wait := kindCreateWait(); wait != "" {
		args = append(args, "--wait", wait)
	}
	if err := runCmd(ctx, h.t, "", nil, "kind", args...); err != nil {
		h.cleanupStaleKindNode(ctx)
		return err
	}
	h.clusterCreated = true
	return nil
}

func (h *Harness) cleanupStaleKindNode(ctx context.Context) {
	name := clusterName + "-control-plane"
	out, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if strings.Contains(trimmed, "No such container") {
			return
		}
		h.t.Logf("[harness] stale kind node cleanup %s failed: %v: %s", name, err, trimmed)
		return
	}
	if err == nil && len(out) > 0 {
		h.t.Logf("[harness] removed stale kind node %s", name)
	}
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
