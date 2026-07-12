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

func TestChartInstallRefFallsBackWhenLocalPackageMissing(t *testing.T) {
	packageDir := t.TempDir()
	t.Setenv(chartPackageDirEnv, packageDir)

	got, local, err := chartInstallRef(config.ChartSpec{Name: "cert-manager", Version: "v1.21.0"})
	if err != nil {
		t.Fatalf("chartInstallRef() error = %v, want nil", err)
	}
	if local {
		t.Fatal("chartInstallRef() local = true, want false")
	}
	if want := chartRegistry + "/cert-manager"; got != want {
		t.Fatalf("chartInstallRef() = %q, want %q", got, want)
	}
}
