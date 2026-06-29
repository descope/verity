package config

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestTempoImagePinsChartCompatiblePackage(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating test file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	def, err := LoadImage(filepath.Join(repoRoot, "images", "tempo.yaml"))
	if err != nil {
		t.Fatalf("load tempo image: %v", err)
	}
	if err := Validate(def); err != nil {
		t.Fatalf("validate tempo image: %v", err)
	}
	if _, ok := def.Versions["2.9.0"]; ok {
		t.Fatalf("tempo versions=%v must not publish vulnerable 2.9.0", def.Versions)
	}
	if _, ok := def.Versions["2.10.0"]; !ok {
		t.Fatalf("tempo versions=%v, want 2.10.0", def.Versions)
	}
	packages := def.Types["default"].Packages
	if !slices.Contains(packages, "tempo~{{version}}") {
		t.Fatalf("tempo packages=%v, want Wolfi package constrained to requested Tempo stream", packages)
	}
}
