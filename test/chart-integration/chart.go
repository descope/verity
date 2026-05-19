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
	helmInstallTimeout            = 10 * time.Minute
	certManagerHelmInstallTimeout = 15 * time.Minute
	chartRegistry                 = "oci://ghcr.io/verity-org/charts"
)

var chartPrerequisites = map[string][]string{
	"cert-manager-csi-driver": {"cert-manager"},
}

var chartFixtures = map[string][]string{
	"cluster-autoscaler": {
		filepath.Join("test", "chart-integration", "fixtures", "cluster-autoscaler", "capi-crds.yaml"),
		filepath.Join("test", "chart-integration", "fixtures", "cluster-autoscaler", "capi-objects.yaml"),
	},
}

var chartFixtureCRDs = map[string][]string{
	"cluster-autoscaler": {
		"clusters.cluster.x-k8s.io",
		"machinedeployments.cluster.x-k8s.io",
		"machinesets.cluster.x-k8s.io",
		"machines.cluster.x-k8s.io",
	},
}

type ChartContext struct {
	Spec      config.ChartSpec
	Namespace string
	ValuesDir string
}

func InstallChart(ctx context.Context, h *Harness, spec config.ChartSpec, valuesDir string) (*ChartContext, error) {
	if err := discovery.ValidateChartSpec(spec); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}
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

func InstallChartPrerequisites(ctx context.Context, h *Harness, spec config.ChartSpec, valuesDir string) ([]*ChartContext, error) {
	prereqNames := chartPrerequisites[spec.Name]
	if len(prereqNames) == 0 {
		return nil, nil
	}

	charts, err := loadChartList(h.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("load chart list for prerequisites: %w", err)
	}
	byName := make(map[string]config.ChartSpec, len(charts))
	for _, candidate := range charts {
		byName[candidate.Name] = candidate
	}
	installed := make([]*ChartContext, 0, len(prereqNames))
	for _, name := range prereqNames {
		candidate, ok := byName[name]
		if !ok {
			return installed, fmt.Errorf("prerequisite %q for %q not found in Chart.yaml", name, spec.Name)
		}
		h.t.Logf("[prereq] installing %s before %s", name, spec.Name)
		cc, installErr := InstallChartWithRetry(ctx, h, candidate, valuesDir, defaultRetryConfig())
		installed = append(installed, cc)
		if installErr != nil {
			return installed, fmt.Errorf("install %s prerequisite: %w", name, installErr)
		}
	}
	return installed, nil
}

func InstallChartFixtures(ctx context.Context, h *Harness, chartName string) error {
	fixtures := chartFixtures[chartName]
	if len(fixtures) == 0 {
		return nil
	}
	for i, fixture := range fixtures {
		path := filepath.Join(h.RepoRoot, fixture)
		h.t.Logf("[fixture] applying %s for %s", path, chartName)
		if err := runCmd(ctx, h.t, "", nil,
			"kubectl", "--kubeconfig", h.KubeconfigPath,
			"apply", "--validate=false", "-f", path,
		); err != nil {
			return fmt.Errorf("apply fixture %s: %w", fixture, err)
		}
		if i == 0 {
			for _, crd := range chartFixtureCRDs[chartName] {
				if err := runCmd(ctx, h.t, "", nil,
					"kubectl", "--kubeconfig", h.KubeconfigPath,
					"wait", "--for=condition=Established", "crd/"+crd, "--timeout=60s",
				); err != nil {
					return fmt.Errorf("wait for fixture CRD %s: %w", crd, err)
				}
			}
		}
	}
	return nil
}

func UninstallChartFixtures(ctx context.Context, h *Harness, chartName string) {
	fixtures := chartFixtures[chartName]
	for i := len(fixtures) - 1; i >= 0; i-- {
		path := filepath.Join(h.RepoRoot, fixtures[i])
		if err := runCmd(ctx, h.t, "", nil,
			"kubectl", "--kubeconfig", h.KubeconfigPath,
			"delete", "-f", path, "--ignore-not-found=true", "--wait=true", "--timeout=120s",
		); err != nil {
			h.t.Logf("[fixture] cleanup %s failed (continuing): %v", path, err)
		}
	}
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
	timeout := chartHelmInstallTimeout(cc.Spec.Name)
	args := []string{
		"--kubeconfig", h.KubeconfigPath,
		"install", cc.Spec.Name,
		chartRegistry + "/" + cc.Spec.Name,
		"--version", cc.Spec.Version,
		"--namespace", cc.Namespace,
		"--create-namespace",
		"--wait",
		"--timeout", timeout.String(),
	}
	if vals := valuesFile(cc); vals != "" {
		args = append(args, "--values", vals)
		h.t.Logf("[install] applying values fixture %s", vals)
	}
	ictx, cancel := context.WithTimeout(ctx, timeout+2*time.Minute)
	defer cancel()
	return runCmd(ictx, h.t, "", []string{"HELM_EXPERIMENTAL_OCI=1"}, "helm", args...)
}

func chartHelmInstallTimeout(chartName string) time.Duration {
	if chartName == "cert-manager" {
		return certManagerHelmInstallTimeout
	}
	return helmInstallTimeout
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
