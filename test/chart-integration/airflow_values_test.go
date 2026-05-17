//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAirflowPostgresqlImageOverrideIsComposable(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating test file")
	}
	valuesPath := filepath.Join(filepath.Dir(file), "values", "airflow.yaml")
	body, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("read %s: %v", valuesPath, err)
	}

	var values struct {
		Airflow struct {
			Postgresql struct {
				Image struct {
					Registry   string `yaml:"registry"`
					Repository string `yaml:"repository"`
				} `yaml:"image"`
			} `yaml:"postgresql"`
		} `yaml:"airflow"`
	}
	if err := yaml.Unmarshal(body, &values); err != nil {
		t.Fatalf("parse %s: %v", valuesPath, err)
	}
	if values.Airflow.Postgresql.Image.Registry != "ghcr.io" {
		t.Fatalf("registry=%q want ghcr.io", values.Airflow.Postgresql.Image.Registry)
	}
	if values.Airflow.Postgresql.Image.Repository != "verity-org/bitnamilegacy/postgresql" {
		t.Fatalf("repository=%q want verity-org/bitnamilegacy/postgresql", values.Airflow.Postgresql.Image.Repository)
	}
}
