//go:build integration

package integration

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGiteaNcShimReportsConnectionFailure(t *testing.T) {
	script := loadGiteaHelperScript(t, "nc")
	path := filepath.Join(t.TempDir(), "nc")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write nc shim: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	if err := exec.Command(path, "-vz", "-w1", "127.0.0.1", strconv.Itoa(addr.Port)).Run(); err != nil {
		listener.Close()
		t.Fatalf("nc shim against listening port failed: %v", err)
	}
	listener.Close()

	if err := exec.Command(path, "-vz", "-w1", "127.0.0.1", strconv.Itoa(addr.Port)).Run(); err == nil {
		t.Fatalf("nc shim succeeded against closed port; configure-gitea would skip its valkey wait loop")
	}
}

func loadGiteaHelperScript(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating test file")
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(file), "values", "gitea.yaml"))
	if err != nil {
		t.Fatalf("read gitea values: %v", err)
	}
	var values struct {
		Gitea struct {
			ExtraDeploy []struct {
				Data map[string]string `yaml:"data"`
			} `yaml:"extraDeploy"`
		} `yaml:"gitea"`
	}
	if err := yaml.Unmarshal(body, &values); err != nil {
		t.Fatalf("parse gitea values: %v", err)
	}
	for _, deploy := range values.Gitea.ExtraDeploy {
		if script := deploy.Data[name]; script != "" {
			return script
		}
	}
	t.Fatalf("gitea helper %q not found", name)
	return ""
}
