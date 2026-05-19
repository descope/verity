package config

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestAirflowImageExposesConsoleScriptsOnPath(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating test file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	def, err := LoadImage(filepath.Join(repoRoot, "images", "airflow.yaml"))
	if err != nil {
		t.Fatalf("load airflow image: %v", err)
	}
	if err := Validate(def); err != nil {
		t.Fatalf("validate airflow image: %v", err)
	}

	tmpl := def.Types["default"]
	if tmpl.Entrypoint != "" {
		t.Fatalf("entrypoint=%q, want empty so Helm chart args become the process command", tmpl.Entrypoint)
	}
	if !slices.Contains(tmpl.Packages, "bash") {
		t.Fatalf("packages=%v must include bash for upstream Helm chart bash -c commands", tmpl.Packages)
	}
	path := tmpl.Environment["PATH"]
	if !pathHasEntry(path, "/opt/airflow/bin") {
		t.Fatalf("PATH=%q must include /opt/airflow/bin for chart bash -c airflow commands", path)
	}

	foundSymlink := false
	for _, p := range tmpl.Paths {
		if p.Path == "/usr/bin/airflow" && p.Type == "symlink" && p.Source == "/opt/airflow/bin/airflow" {
			foundSymlink = true
			break
		}
	}
	if !foundSymlink {
		t.Fatal("missing /usr/bin/airflow -> /opt/airflow/bin/airflow symlink")
	}
}

func pathHasEntry(path, want string) bool {
	return slices.Contains(strings.Split(path, ":"), want)
}
