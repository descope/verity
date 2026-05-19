//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
			DefaultAirflowRepository string `yaml:"defaultAirflowRepository"`
			DefaultAirflowTag        string `yaml:"defaultAirflowTag"`
			Images                   struct {
				MigrationsWaitTimeout int `yaml:"migrationsWaitTimeout"`
			} `yaml:"images"`
			Postgresql               struct {
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
	if values.Airflow.DefaultAirflowRepository != "ghcr.io/verity-org/airflow" {
		t.Fatalf("defaultAirflowRepository=%q want ghcr.io/verity-org/airflow", values.Airflow.DefaultAirflowRepository)
	}
	if values.Airflow.DefaultAirflowTag != "3" {
		t.Fatalf("defaultAirflowTag=%q want 3", values.Airflow.DefaultAirflowTag)
	}
	if values.Airflow.Images.MigrationsWaitTimeout < 900 {
		t.Fatalf("migrationsWaitTimeout=%d want at least 900", values.Airflow.Images.MigrationsWaitTimeout)
	}
	if values.Airflow.Postgresql.Image.Registry != "ghcr.io" {
		t.Fatalf("registry=%q want ghcr.io", values.Airflow.Postgresql.Image.Registry)
	}
	if values.Airflow.Postgresql.Image.Repository != "verity-org/bitnamilegacy/postgresql" {
		t.Fatalf("repository=%q want verity-org/bitnamilegacy/postgresql", values.Airflow.Postgresql.Image.Repository)
	}
}

func TestAirflowHasExtendedHelmInstallTimeout(t *testing.T) {
	if got, want := chartHelmInstallTimeout("airflow"), 20*time.Minute; got != want {
		t.Fatalf("chartHelmInstallTimeout(airflow)=%s want %s", got, want)
	}
}
