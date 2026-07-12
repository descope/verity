//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

func TestChartInstallRefUsesLocalPackageDir(t *testing.T) {
	packageDir := t.TempDir()
	spec := config.ChartSpec{Name: "cert-manager", Version: "v1.21.0"}
	packagePath := filepath.Join(packageDir, "cert-manager-v1.21.0.tgz")
	if err := os.WriteFile(packagePath, []byte("chart"), 0o644); err != nil {
		t.Fatalf("write chart package: %v", err)
	}
	t.Setenv(chartPackageDirEnv, packageDir)

	got, local, err := chartInstallRef(spec)
	if err != nil {
		t.Fatalf("chartInstallRef() error = %v", err)
	}
	if !local {
		t.Fatal("chartInstallRef() local = false, want true")
	}
	if got != packagePath {
		t.Fatalf("chartInstallRef() = %q, want %q", got, packagePath)
	}
}

func TestChartInstallRefReportsMissingLocalPackage(t *testing.T) {
	packageDir := t.TempDir()
	t.Setenv(chartPackageDirEnv, packageDir)

	_, _, err := chartInstallRef(config.ChartSpec{Name: "cert-manager", Version: "v1.21.0"})
	if err == nil {
		t.Fatal("chartInstallRef() error = nil, want missing package error")
	}
}
