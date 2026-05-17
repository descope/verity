//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMimirDistributedSmokeTestEndpointsBypassDisabledGateway(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating test file")
	}
	valuesPath := filepath.Join(filepath.Dir(file), "values", "mimir-distributed.yaml")
	body, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("read %s: %v", valuesPath, err)
	}

	var values struct {
		MimirDistributed struct {
			SmokeTest struct {
				ExtraArgs map[string]string `yaml:"extraArgs"`
				TenantID  string            `yaml:"tenantId"`
			} `yaml:"smoke_test"`
		} `yaml:"mimir-distributed"`
	}
	if err := yaml.Unmarshal(body, &values); err != nil {
		t.Fatalf("parse %s: %v", valuesPath, err)
	}
	if values.MimirDistributed.SmokeTest.TenantID != "anonymous" {
		t.Fatalf("smoke_test.tenantId=%q want anonymous", values.MimirDistributed.SmokeTest.TenantID)
	}

	want := map[string]string{
		"tests.write-endpoint": "http://mimir-distributed-distributor.mimir-distributed.svc:8080",
		"tests.read-endpoint":  "http://mimir-distributed-query-frontend.mimir-distributed.svc:8080/prometheus",
	}
	for key, wantValue := range want {
		if got := values.MimirDistributed.SmokeTest.ExtraArgs[key]; got != wantValue {
			t.Errorf("smoke_test.extraArgs[%q]=%q want %q", key, got, wantValue)
		}
	}
}
