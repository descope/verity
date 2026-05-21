package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestRabbitMQClusterOperatorImageHasChartManagerPath(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating test file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	def, err := LoadImage(filepath.Join(repoRoot, "images", "rabbitmq-cluster-operator.yaml"))
	if err != nil {
		t.Fatalf("load rabbitmq-cluster-operator image: %v", err)
	}
	if err := Validate(def); err != nil {
		t.Fatalf("validate rabbitmq-cluster-operator image: %v", err)
	}

	tmpl := def.Types["default"]
	if tmpl.Entrypoint != "/usr/bin/manager" {
		t.Fatalf("entrypoint=%q, want /usr/bin/manager", tmpl.Entrypoint)
	}
	manager, ok := findPath(tmpl.Paths, "/usr/bin/manager")
	if !ok {
		t.Fatal("missing /usr/bin/manager permissions entry")
	}
	if manager.Type != "permissions" || manager.Permissions != "0o755" {
		t.Fatalf("/usr/bin/manager path entry = type:%q mode:%q, want permissions 0o755", manager.Type, manager.Permissions)
	}

	found := false
	for _, p := range tmpl.Paths {
		if p.Path == "/manager" && p.Type == "symlink" && p.Source == "/usr/bin/manager" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing /manager -> /usr/bin/manager symlink required by chart command")
	}
}
