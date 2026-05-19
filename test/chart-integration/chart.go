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

type chartPrerequisite struct {
	Name          string
	SameNamespace bool
}

var chartPrerequisites = map[string][]chartPrerequisite{
	"cert-manager-csi-driver": {{Name: "cert-manager"}},
	// OpenSearch Dashboards blocks startup until it can query the default
	// opensearch-cluster-master service. Install the OpenSearch prerequisite in
	// the dashboard namespace so the chart default resolves during the smoke test.
	"opensearch-dashboards": {{Name: "opensearch", SameNamespace: true}},
}

var chartCRDFixtures = map[string][]string{
	"cluster-autoscaler": {
		filepath.Join("test", "chart-integration", "fixtures", "cluster-autoscaler", "capi-crds.yaml"),
	},
}

var chartObjectFixtures = map[string][]string{
	"cluster-autoscaler": {
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
	Spec              config.ChartSpec
	Namespace         string
	ValuesDir         string
	PreserveNamespace bool
}

func InstallChart(ctx context.Context, h *Harness, spec config.ChartSpec, valuesDir string) (*ChartContext, error) {
	return installChartInNamespace(ctx, h, spec, valuesDir, sanitizeNamespace(spec.Name), false)
}

func installChartInNamespace(ctx context.Context, h *Harness, spec config.ChartSpec, valuesDir, namespace string, preserveNamespace bool) (*ChartContext, error) {
	if err := discovery.ValidateChartSpec(spec); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}
	cc := &ChartContext{
		Spec:              spec,
		Namespace:         namespace,
		ValuesDir:         valuesDir,
		PreserveNamespace: preserveNamespace,
	}
	if err := helmInstall(ctx, h, cc); err != nil {
		return cc, fmt.Errorf("helm install: %w", err)
	}
	return cc, nil
}

func InstallChartPrerequisites(ctx context.Context, h *Harness, spec config.ChartSpec, valuesDir string) ([]*ChartContext, error) {
	prereqs := chartPrerequisites[spec.Name]
	if len(prereqs) == 0 {
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
	installed := make([]*ChartContext, 0, len(prereqs))
	for _, prereq := range prereqs {
		candidate, ok := byName[prereq.Name]
		if !ok {
			return installed, fmt.Errorf("prerequisite %q for %q not found in Chart.yaml", prereq.Name, spec.Name)
		}
		namespace := sanitizeNamespace(candidate.Name)
		if prereq.SameNamespace {
			namespace = sanitizeNamespace(spec.Name)
		}
		h.t.Logf("[prereq] installing %s before %s in namespace %s", prereq.Name, spec.Name, namespace)
		cc, installErr := installChartWithRetryInNamespace(ctx, h, candidate, valuesDir, namespace, false, defaultRetryConfig())
		installed = append(installed, cc)
		if installErr != nil {
			return installed, fmt.Errorf("install %s prerequisite: %w", prereq.Name, installErr)
		}
	}
	return installed, nil
}

func InstallChartFixtures(ctx context.Context, h *Harness, chartName string) error {
	crdFixtures := chartCRDFixtures[chartName]
	objectFixtures := chartObjectFixtures[chartName]
	if len(crdFixtures) == 0 && len(objectFixtures) == 0 {
		return nil
	}
	for _, fixture := range crdFixtures {
		path := filepath.Join(h.RepoRoot, fixture)
		h.t.Logf("[fixture] applying %s for %s", path, chartName)
		if err := runCmd(ctx, h.t, "", nil,
			"kubectl", "--kubeconfig", h.KubeconfigPath,
			"apply", "--validate=false", "-f", path,
		); err != nil {
			return fmt.Errorf("apply fixture %s: %w", fixture, err)
		}
	}
	for _, crd := range chartFixtureCRDs[chartName] {
		if err := runCmd(ctx, h.t, "", nil,
			"kubectl", "--kubeconfig", h.KubeconfigPath,
			"wait", "--for=condition=Established", "crd/"+crd, "--timeout=60s",
		); err != nil {
			return fmt.Errorf("wait for fixture CRD %s: %w", crd, err)
		}
	}
	for _, fixture := range objectFixtures {
		path := filepath.Join(h.RepoRoot, fixture)
		h.t.Logf("[fixture] applying %s for %s", path, chartName)
		if err := runCmd(ctx, h.t, "", nil,
			"kubectl", "--kubeconfig", h.KubeconfigPath,
			"apply", "--validate=false", "-f", path,
		); err != nil {
			return fmt.Errorf("apply fixture %s: %w", fixture, err)
		}
	}
	if err := seedChartFixtureStatus(ctx, h, chartName); err != nil {
		return err
	}
	return nil
}

func seedChartFixtureStatus(ctx context.Context, h *Harness, chartName string) error {
	switch chartName {
	case "cluster-autoscaler":
		// Normal kubectl apply cannot write status when the CRD exposes a status
		// subresource. Seed the scale paths that cluster-autoscaler reads from
		// the MachineDeployment /scale endpoint; no CAPI controller runs in this
		// bounded smoke fixture to populate them for us.
		if err := runCmd(ctx, h.t, "", nil,
			"kubectl", "--kubeconfig", h.KubeconfigPath,
			"-n", "cluster-autoscaler",
			"patch", "machinedeployment.cluster.x-k8s.io/verity-it-smoke-md",
			"--subresource=status",
			"--type=merge",
			"-p", `{"status":{"replicas":0,"selector":"cluster.x-k8s.io/cluster-name=verity-it,cluster.x-k8s.io/deployment-name=verity-it-smoke-md"}}`,
		); err != nil {
			return fmt.Errorf("seed cluster-autoscaler fixture status: %w", err)
		}
	}
	return nil
}

func UninstallChartFixtures(ctx context.Context, h *Harness, chartName string) {
	fixtures := append([]string{}, chartCRDFixtures[chartName]...)
	fixtures = append(fixtures, chartObjectFixtures[chartName]...)
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
	if cc.PreserveNamespace {
		return
	}
	if err := runCmd(ctx, h.t, "", nil,
		"kubectl", "--kubeconfig", h.KubeconfigPath,
		"delete", "namespace", cc.Namespace, "--ignore-not-found", "--wait=false",
	); err != nil {
		h.t.Logf("[uninstall] kubectl delete ns %s: %v (continuing)", cc.Namespace, err)
	}
}

func preserveNamespaceOnRetry(chartName string) bool {
	for _, prereq := range chartPrerequisites[chartName] {
		if prereq.SameNamespace {
			return true
		}
	}
	return false
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
