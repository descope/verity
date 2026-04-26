package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestVerityConfigChartImageOverridesYAML(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "verity-chart-image-overrides.yaml"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	var cfg VerityConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	if len(cfg.ChartImageOverrides) != 2 {
		t.Fatalf("len(ChartImageOverrides) = %d, want 2", len(cfg.ChartImageOverrides))
	}

	strimzi := cfg.ChartImageOverrides["strimzi-kafka-operator"]
	if len(strimzi) != 2 {
		t.Fatalf("len(strimzi overrides) = %d, want 2", len(strimzi))
	}
	if strimzi[0].Source != "STRIMZI_DEFAULT_KAFKA_EXPORTER_IMAGE" || strimzi[0].Path != "kafka.image" || strimzi[0].Type != "" {
		t.Fatalf("strimzi[0] = %+v, want source/path with default type", strimzi[0])
	}
	if strimzi[1].Source != "STRIMZI_KAFKA_IMAGES" || strimzi[1].Type != "csv" || strimzi[1].Path != "kafka.versions[\"{version}\"]" {
		t.Fatalf("strimzi[1] = %+v, want csv override", strimzi[1])
	}

	rook := cfg.ChartImageOverrides["rook-ceph"]
	if len(rook) != 2 {
		t.Fatalf("len(rook overrides) = %d, want 2", len(rook))
	}
	if rook[0].Source != "ROOK_CSI_CEPH_IMAGE" || rook[0].Path != "csi.cephcsi.repository" {
		t.Fatalf("rook[0] = %+v, want ceph CSI override", rook[0])
	}
	if rook[1].Source != "ROOK_CSI_PROVISIONER_IMAGE" || rook[1].Path != "csi.provisionerImage" {
		t.Fatalf("rook[1] = %+v, want provisioner override", rook[1])
	}
}
