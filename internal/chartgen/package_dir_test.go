package chartgen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

func TestMovePackagedChartWritesIntoPackageDir(t *testing.T) {
	srcDir := t.TempDir()
	packageDir := t.TempDir()
	src := filepath.Join(srcDir, "cert-manager-v1.21.0.tgz")
	if err := os.WriteFile(src, []byte("chart"), 0o644); err != nil {
		t.Fatalf("write source package: %v", err)
	}

	dest, err := movePackagedChart(src, packageDir)
	if err != nil {
		t.Fatalf("movePackagedChart() error = %v", err)
	}

	want := filepath.Join(packageDir, "cert-manager-v1.21.0.tgz")
	if dest != want {
		t.Fatalf("movePackagedChart() = %q, want %q", dest, want)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source package still exists or stat failed differently: %v", err)
	}
	if got, err := os.ReadFile(want); err != nil || string(got) != "chart" {
		t.Fatalf("dest package = %q, %v; want chart", got, err)
	}
}

func TestSelectChartsKeepsOnlyRequestedChart(t *testing.T) {
	charts := []config.ChartSpec{
		{Name: "prometheus", Version: "29.14.0"},
		{Name: "cert-manager", Version: "v1.21.0"},
	}

	got, err := selectCharts(charts, "cert-manager")
	if err != nil {
		t.Fatalf("selectCharts() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "cert-manager" || got[0].Version != "v1.21.0" {
		t.Fatalf("selectCharts() = %#v, want only cert-manager v1.21.0", got)
	}
}

func TestSelectChartsRejectsUnknownChart(t *testing.T) {
	_, err := selectCharts([]config.ChartSpec{{Name: "prometheus"}}, "cert-manager")
	if err == nil {
		t.Fatal("selectCharts() error = nil, want unknown chart error")
	}
}
