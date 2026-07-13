package cmd

import (
	"errors"
	"testing"

	"github.com/verity-org/verity/internal/chartgen"
)

func TestValidateChartGenConfigRequiresChartRegistryForPush(t *testing.T) {
	err := validateChartGenConfig(&chartgen.Config{})
	if !errors.Is(err, errChartGenMissingChartRegistry) {
		t.Fatalf("validateChartGenConfig() error = %v, want errChartGenMissingChartRegistry", err)
	}
}

func TestValidateChartGenConfigAllowsPackageDirWithoutChartRegistry(t *testing.T) {
	err := validateChartGenConfig(&chartgen.Config{PackageDir: t.TempDir()})
	if err != nil {
		t.Fatalf("validateChartGenConfig() error = %v, want nil", err)
	}
}

func TestValidateChartGenConfigAllowsChartRegistry(t *testing.T) {
	err := validateChartGenConfig(&chartgen.Config{ChartRegistry: "oci://ghcr.io/verity-org/charts"})
	if err != nil {
		t.Fatalf("validateChartGenConfig() error = %v, want nil", err)
	}
}
