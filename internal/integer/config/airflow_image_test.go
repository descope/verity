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
	if !slices.Contains(tmpl.Packages, "busybox") {
		t.Fatalf("packages=%v must include busybox for upstream Helm chart sh-based exec probes", tmpl.Packages)
	}
	path := tmpl.Environment["PATH"]
	if !pathHasEntry(path, "/opt/airflow/bin") {
		t.Fatalf("PATH=%q must include /opt/airflow/bin for chart bash -c airflow commands", path)
	}
	if tmpl.Environment["USER"] != "airflow" {
		t.Fatalf("USER=%q want airflow so Airflow CLI getpass.getuser works under chart uid 50000", tmpl.Environment["USER"])
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

	for _, dirPath := range []string{"/opt/airflow", "/opt/airflow/dags", "/opt/airflow/logs", "/opt/airflow/plugins"} {
		p, ok := findPath(tmpl.Paths, dirPath)
		if !ok {
			t.Fatalf("missing %s path entry", dirPath)
		}
		if p.UID != 50000 || p.GID != 0 || p.Permissions != "0o775" {
			t.Fatalf("%s ownership/perms = uid:%d gid:%d mode:%s, want uid:50000 gid:0 mode:0o775", dirPath, p.UID, p.GID, p.Permissions)
		}
	}
}

func pathHasEntry(path, want string) bool {
	return slices.Contains(strings.Split(path, ":"), want)
}

func findPath(paths []PathDef, want string) (PathDef, bool) {
	for _, p := range paths {
		if p.Path == want {
			return p, true
		}
	}
	return PathDef{}, false
}
