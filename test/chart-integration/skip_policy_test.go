//go:build integration

package integration

import "testing"

func TestShouldSkipChartHonorsSKIPSByDefault(t *testing.T) {
	// Given
	cfg, err := LoadSkips(writeSkips(t, validSkipsYAML))
	if err != nil {
		t.Fatalf("LoadSkips: %v", err)
	}

	// When
	hit, entry := shouldSkipChart(cfg, "falco")

	// Then
	if !hit {
		t.Fatal("shouldSkipChart(falco) = false, want true")
	}
	if entry == nil || entry.Chart != "falco" {
		t.Fatalf("skip entry = %#v, want falco entry", entry)
	}
}

func TestShouldSkipChartOverrideRunsOnlyListedSkippedCharts(t *testing.T) {
	// Given
	t.Setenv(runSkippedChartsEnv, " falco , cilium ")
	cfg, err := LoadSkips(writeSkips(t, validSkipsYAML))
	if err != nil {
		t.Fatalf("LoadSkips: %v", err)
	}

	// When
	hit, entry := shouldSkipChart(cfg, "falco")

	// Then
	if hit || entry != nil {
		t.Fatalf("shouldSkipChart(falco) = (%v, %#v), want (false, nil)", hit, entry)
	}
}

func TestShouldSkipChartOverrideDoesNotAffectUnlistedSkips(t *testing.T) {
	// Given
	t.Setenv(runSkippedChartsEnv, "cilium")
	cfg, err := LoadSkips(writeSkips(t, validSkipsYAML))
	if err != nil {
		t.Fatalf("LoadSkips: %v", err)
	}

	// When
	hit, entry := shouldSkipChart(cfg, "falco")

	// Then
	if !hit {
		t.Fatal("shouldSkipChart(falco) = false, want true")
	}
	if entry == nil || entry.Chart != "falco" {
		t.Fatalf("skip entry = %#v, want falco entry", entry)
	}
}
