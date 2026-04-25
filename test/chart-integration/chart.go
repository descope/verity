//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

const (
	helmInstallTimeout = 10 * time.Minute
	chartRegistry      = "oci://ghcr.io/verity-org/charts"
)

type ChartContext struct {
	Spec      config.ChartSpec
	Namespace string
	ValuesDir string
}

func InstallChart(ctx context.Context, h *Harness, spec config.ChartSpec, valuesDir string) (*ChartContext, error) {
	cc := &ChartContext{
		Spec:      spec,
		Namespace: sanitizeNamespace(spec.Name),
		ValuesDir: valuesDir,
	}
	if err := helmInstall(ctx, h, cc); err != nil {
		return cc, fmt.Errorf("helm install: %w", err)
	}
	return cc, nil
}

func UninstallChart(ctx context.Context, h *Harness, cc *ChartContext) {
	if cc == nil {
		return
	}
	if err := runCmd(ctx, h.t, "", nil,
		"helm", "--kubeconfig", h.KubeconfigPath,
		"uninstall", cc.Spec.Name, "--namespace", cc.Namespace, "--ignore-not-found",
	); err != nil {
		h.t.Logf("[uninstall] helm uninstall %s: %v (continuing)", cc.Spec.Name, err)
	}
	if err := runCmd(ctx, h.t, "", nil,
		"kubectl", "--kubeconfig", h.KubeconfigPath,
		"delete", "namespace", cc.Namespace, "--ignore-not-found", "--wait=false",
	); err != nil {
		h.t.Logf("[uninstall] kubectl delete ns %s: %v (continuing)", cc.Namespace, err)
	}
}

func helmInstall(ctx context.Context, h *Harness, cc *ChartContext) error {
	args := []string{
		"--kubeconfig", h.KubeconfigPath,
		"install", cc.Spec.Name,
		chartRegistry + "/" + cc.Spec.Name,
		"--version", cc.Spec.Version,
		"--namespace", cc.Namespace,
		"--create-namespace",
		"--wait",
		"--timeout", "10m",
	}
	if vals := valuesFile(cc); vals != "" {
		args = append(args, "--values", vals)
		h.t.Logf("[install] applying values fixture %s", vals)
	}
	ictx, cancel := context.WithTimeout(ctx, helmInstallTimeout+2*time.Minute)
	defer cancel()
	return runCmd(ictx, h.t, "", []string{"HELM_EXPERIMENTAL_OCI=1"}, "helm", args...)
}

func HelmTest(ctx context.Context, h *Harness, cc *ChartContext) error {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, err := runCmdOutput(tctx, "", nil,
		"helm", "--kubeconfig", h.KubeconfigPath,
		"test", cc.Spec.Name, "--namespace", cc.Namespace,
	)
	if err != nil && strings.Contains(out, "no tests to run") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("helm test: %s: %w", out, err)
	}
	return nil
}

func valuesFile(cc *ChartContext) string {
	candidate := filepath.Join(cc.ValuesDir, cc.Spec.Name+".yaml")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func sanitizeNamespace(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func DumpDiagnostics(ctx context.Context, h *Harness, label string) {
	if h.KubeconfigPath == "" {
		return
	}
	dumpDir := filepath.Join(h.RepoRoot, "_dump-"+label)
	if err := os.MkdirAll(dumpDir, 0o755); err != nil {
		h.t.Logf("[diag] mkdir %s: %v", dumpDir, err)
		return
	}
	dumpCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", h.KubeconfigPath,
		"cluster-info", "dump", "--all-namespaces",
		"--output-directory="+dumpDir)
	if err := dumpCmd.Run(); err != nil {
		h.t.Logf("[diag] cluster-info dump: %v", err)
	}
	logsCmd := exec.CommandContext(ctx, "kind", "export", "logs",
		filepath.Join(h.RepoRoot, "_kind-logs-"+label),
		"--name", clusterName)
	if err := logsCmd.Run(); err != nil {
		h.t.Logf("[diag] kind export logs: %v", err)
	}
}

func loadChartList(repoRoot string) ([]config.ChartSpec, error) {
	return discovery.LoadChartsFile(filepath.Join(repoRoot, "Chart.yaml"))
}
