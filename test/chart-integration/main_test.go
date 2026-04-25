//go:build integration

package integration

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/verity-org/verity/internal/config"
)

var (
	testHarness *Harness
	repoRoot    string
	valuesDir   string
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(testMainImpl(m))
}

func testMainImpl(m *testing.M) int {
	root, rootErr := findRepoRoot()
	if rootErr != nil {
		fmt.Fprintf(os.Stderr, "locating repo root: %v\n", rootErr)
		return 1
	}
	repoRoot = root
	valuesDir = filepath.Join(repoRoot, "test", "chart-integration", "values")

	if os.Getenv("VERITY_IT_SKIP_CLUSTER") == "1" {
		return m.Run()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	h := NewHarness(stdoutLogger{}, repoRoot)
	if err := h.Setup(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[harness setup] %v\n", err)
		tctx, tcancel := context.WithTimeout(context.Background(), 3*time.Minute)
		h.Teardown(tctx)
		tcancel()
		return 1
	}
	testHarness = h

	code := m.Run()

	tctx, tcancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer tcancel()
	h.Teardown(tctx)
	return code
}

func TestCharts(t *testing.T) {
	if testHarness == nil {
		t.Skip("cluster harness disabled (VERITY_IT_SKIP_CLUSTER=1)")
	}
	chartYamlPath := filepath.Join(repoRoot, "Chart.yaml")
	if _, err := os.Stat(chartYamlPath); err != nil {
		t.Fatalf("Chart.yaml not found at %s: %v (broken repo-root lookup or checkout?)", chartYamlPath, err)
	}
	charts, err := loadChartList(repoRoot)
	if err != nil {
		t.Fatalf("load Chart.yaml: %v", err)
	}
	if len(charts) == 0 {
		t.Fatalf("Chart.yaml at %s has no dependencies — smoke test would silently pass with zero charts run", chartYamlPath)
	}
	filter := strings.TrimSpace(os.Getenv("VERITY_CHART"))
	matched := 0
	for _, spec := range charts {
		if filter != "" && spec.Name != filter {
			continue
		}
		matched++
		t.Run(spec.Name, func(t *testing.T) {
			runChart(t, spec)
		})
	}
	if filter != "" && matched == 0 {
		t.Fatalf("VERITY_CHART=%q matched no chart in Chart.yaml", filter)
	}
}

func runChart(t *testing.T, spec config.ChartSpec) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	cc, installErr := InstallChart(ctx, testHarness, spec, valuesDir)
	defer func() {
		if t.Failed() {
			dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Minute)
			DumpDiagnostics(dctx, testHarness, spec.Name)
			dcancel()
		}
		uctx, ucancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer ucancel()
		UninstallChart(uctx, testHarness, cc)
	}()
	if installErr != nil {
		t.Fatalf("install: %v", installErr)
	}

	if err := HelmTest(ctx, testHarness, cc); err != nil {
		t.Errorf("helm test: %v", err)
	}

	WaitSettle(ctx)

	if err := AssertNoRestarts(ctx, testHarness, cc.Namespace); err != nil {
		t.Errorf("no-restart gate: %v", err)
	}

	allow := filepath.Join(valuesDir, spec.Name+".allowlist.txt")
	if err := AssertImageOrigin(ctx, testHarness, cc.Namespace, allow); err != nil {
		t.Errorf("image-origin gate: %v", err)
	}
}

type stdoutLogger struct{}

func (stdoutLogger) Logf(format string, args ...any) {
	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}
	fmt.Fprintf(os.Stdout, format, args...)
}

func (stdoutLogger) Helper() {}

func findRepoRoot() (string, error) {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for range 6 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", os.ErrNotExist
}
