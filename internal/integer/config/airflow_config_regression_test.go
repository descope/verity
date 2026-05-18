package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAirflowImageCarriesChartEntrypointCompatibility(t *testing.T) {
	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	def, err := LoadImage(filepath.Join(repoRoot, "images", "airflow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(def); err != nil {
		t.Fatal(err)
	}

	tmpl := def.Types["default"]
	if tmpl.Entrypoint != "/entrypoint" {
		t.Fatalf("entrypoint=%q want /entrypoint", tmpl.Entrypoint)
	}
	if tmpl.Melange == nil || tmpl.Melange.Bespoke != "airflow-compat-entrypoint.yaml" {
		t.Fatalf("melange=%#v want bespoke airflow-compat-entrypoint.yaml", tmpl.Melange)
	}
	if !slices.Contains(tmpl.Packages, "airflow-compat-entrypoint") {
		t.Fatalf("packages=%v missing airflow-compat-entrypoint", tmpl.Packages)
	}
	if !strings.HasPrefix(tmpl.Environment["PATH"], "/opt/airflow/bin:") {
		t.Fatalf("PATH=%q missing /opt/airflow/bin prefix", tmpl.Environment["PATH"])
	}

	foundSymlink := false
	for _, p := range tmpl.Paths {
		if p.Path == "/usr/bin/airflow" && p.Type == "symlink" && p.Source == "/opt/airflow/bin/airflow" {
			foundSymlink = true
		}
	}
	if !foundSymlink {
		t.Fatalf("missing /usr/bin/airflow -> /opt/airflow/bin/airflow symlink in paths: %#v", tmpl.Paths)
	}
}
