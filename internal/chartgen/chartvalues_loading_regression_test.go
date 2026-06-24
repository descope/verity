package chartgen

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	cfgpkg "github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

// TestVerityConfigChartValuesLoading is a regression guard for the chart-gen
// strict-mode fix landed in PR #386 / #387: charts with verity.yaml
// chartValues entries (gitea) AND charts whose images are all in
// unpatchableImages (victoria-logs-single) must NOT be counted as
// strict-mode skips.
//
// This test ONLY validates the YAML loading path — full processChart
// coverage lives in processchart_emit_test.go.
func TestVerityConfigChartValuesLoading(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(repoRoot, "verity.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var vc cfgpkg.VerityConfig
	if err := yaml.Unmarshal(b, &vc); err != nil {
		t.Fatal(err)
	}
	if gv := vc.ChartValues["gitea"]; len(gv) == 0 {
		t.Fatalf("gitea chartValues missing — chartgen would treat it as zero-mappings skip")
	}

	vc2, err := discovery.LoadVerityConfig(filepath.Join(repoRoot, "verity.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if gv := vc2.ChartValues["gitea"]; len(gv) == 0 {
		t.Fatalf("LoadVerityConfig dropped gitea chartValues — chartgen would skip gitea")
	}
	if got := vc2.ChartValues["fluent-bit"]["livenessProbe.initialDelaySeconds"]; got != 45 {
		t.Fatalf("fluent-bit livenessProbe.initialDelaySeconds = %#v, want 45", got)
	}
	if got := vc2.Overrides["jetstack/cert-manager-csi-driver"].ValuePath; got != "image" {
		t.Fatalf("cert-manager-csi-driver valuePath = %q, want image", got)
	}
	certManagerCSIDriver := vc2.Replacements["jetstack/cert-manager-csi-driver"]
	if certManagerCSIDriver.Tag != "0.15" {
		t.Fatalf("cert-manager-csi-driver replacement tag=%q, want 0.15", certManagerCSIDriver.Tag)
	}
	tempo := vc2.Replacements["grafana/tempo"]
	if tempo.Tag != "2.9.0" {
		t.Fatalf("grafana/tempo replacement tag=%q, want 2.9.0", tempo.Tag)
	}
	airflow := vc2.ChartValues["airflow"]
	if airflow["postgresql.image.registry"] != "" {
		t.Fatalf("airflow postgresql.image.registry=%v, want empty string", airflow["postgresql.image.registry"])
	}
	if airflow["postgresql.image.repository"] != "bitnamilegacy/postgresql" {
		t.Fatalf("airflow postgresql.image.repository=%v, want bitnamilegacy/postgresql", airflow["postgresql.image.repository"])
	}
	valkey := vc2.ChartValues["valkey"]
	if valkey["image.registry"] != "ghcr.io" {
		t.Fatalf("valkey image.registry=%v, want ghcr.io", valkey["image.registry"])
	}
	if valkey["image.repository"] != "verity-org/valkey" {
		t.Fatalf("valkey image.repository=%v, want verity-org/valkey", valkey["image.repository"])
	}
	if valkey["image.tag"] != "9.0" {
		t.Fatalf("valkey image.tag=%v, want 9.0", valkey["image.tag"])
	}
	rabbitmqClusterOperator, ok := vc2.Replacements["rabbitmq/cluster-operator"]
	if !ok {
		t.Fatalf("missing replacement for rabbitmq/cluster-operator")
	}
	if rabbitmqClusterOperator.Registry != "ghcr.io/verity-org" {
		t.Fatalf("rabbitmq/cluster-operator replacement registry=%q, want ghcr.io/verity-org", rabbitmqClusterOperator.Registry)
	}
	if rabbitmqClusterOperator.Image != "rabbitmq-cluster-operator" {
		t.Fatalf("rabbitmq/cluster-operator replacement image=%q, want rabbitmq-cluster-operator", rabbitmqClusterOperator.Image)
	}
	if rabbitmqClusterOperator.Tag != "2" {
		t.Fatalf("rabbitmq/cluster-operator replacement tag=%q, want 2", rabbitmqClusterOperator.Tag)
	}

	// victoria-logs-single has NO chartValues but its only image is in
	// unpatchableImages — the new processChart passthrough branch must
	// handle it without a chartValues entry.
	if !slices.Contains(vc2.UnpatchableImages, "victoriametrics/victoria-logs") {
		t.Fatalf("expected victoriametrics/victoria-logs in unpatchableImages")
	}
}
