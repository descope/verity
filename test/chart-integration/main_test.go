//go:build integration

package integration

import (
	"context"
	"errors"
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
	skipsCfg    *SkipsConfig
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

	// SKIPS.yaml is loaded once per test binary. Missing file is OK
	// (empty skip list). Malformed file is FATAL — silent skips are the
	// worst possible failure mode for a smoke suite (SCR-2026-05-14-001
	// AC-1, AC-3).
	sc, sErr := LoadSkips(filepath.Join(repoRoot, "test", "chart-integration", "SKIPS.yaml"))
	if sErr != nil {
		fmt.Fprintf(os.Stderr, "[SKIPS.yaml] fail-closed: %v\n", sErr)
		return 1
	}
	skipsCfg = sc

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
			if skip, entry := skipsCfg.IsSkipped(spec.Name); skip {
				// Drop a sentinel for the workflow's Record-shard-outcome step
				// so the GitHub Step Summary can render "skipped" distinctly
				// from "success". The step grade gracefully degrades if the
				// write fails — `make chart-integration` still exits 0 and
				// `steps.smoke.outcome` still reports success in that case.
				writeSkipSentinel(repoRoot, spec.Name, entry)
				t.Skipf("SKIP per SKIPS.yaml: chart=%s reason=%s tracking=%s",
					spec.Name, entry.Reason, entry.TrackingIssue)
			}
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

	prereqs, prereqErr := InstallChartPrerequisites(ctx, testHarness, spec, valuesDir)
	defer func() {
		for i := len(prereqs) - 1; i >= 0; i-- {
			uctx, ucancel := context.WithTimeout(context.Background(), 3*time.Minute)
			UninstallChart(uctx, testHarness, prereqs[i])
			ucancel()
		}
	}()
	if prereqErr != nil {
		t.Fatalf("prerequisites: %v", prereqErr)
	}
	defer func() {
		fctx, fcancel := context.WithTimeout(context.Background(), 3*time.Minute)
		UninstallChartFixtures(fctx, testHarness, spec.Name)
		fcancel()
	}()
	if err := InstallChartFixtures(ctx, testHarness, spec.Name); err != nil {
		t.Fatalf("fixtures: %v", err)
	}

	cc, installErr := InstallChartWithRetry(ctx, testHarness, spec, valuesDir, defaultRetryConfig())
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
	err := AssertImageOrigin(ctx, testHarness, cc.Namespace, allow)
	switch {
	case err == nil:
		// allowlist present (or empty file) AND every image accepted
		// by AssertImageOrigin's existing logic — clean pass.
	case errors.Is(err, errAllowlistMissing):
		// SCR-2026-04-30-001 (Option E): missing-allowlist is no
		// longer silently treated as empty-allowlist. Re-collect
		// the namespace's images and hard-fail iff any are
		// non-verity-prefixed. If every image is already verity-
		// prefixed, the absence is a clean pass — the chart
		// genuinely doesn't need an allowlist file.
		images, listErr := CollectNamespaceImages(ctx, testHarness, cc.Namespace)
		if listErr != nil {
			t.Errorf("image-origin gate: allowlist missing AND namespace image collection failed: %v (original error: %v)", listErr, err)
			break
		}
		offenders := classifyMissingAllowlist(images)
		if len(offenders) > 0 {
			t.Errorf("image-origin gate: chart %q has %d non-verity image(s) but no allowlist file at %s — author the allowlist for this chart (SCR-2026-04-30-001):\n  %s",
				spec.Name, len(offenders), allow, strings.Join(offenders, "\n  "))
		}
	default:
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

// writeSkipSentinel drops `_skip-<chart>` at the repo root. The workflow's
// "Record shard outcome" step checks for this file to render a distinct
// "skipped" status in $GITHUB_STEP_SUMMARY — without it, `steps.smoke.outcome`
// would report "success" for skipped charts, which is misleading.
//
// Best-effort: a write failure is logged but does NOT fail the test. The
// alternative (failing the chart that wanted to be skipped) defeats the
// purpose of SKIPS.yaml. Workflow degrades gracefully to plain "success".
func writeSkipSentinel(root, chart string, entry *SkipEntry) {
	if root == "" || entry == nil {
		return
	}
	body := fmt.Sprintf("chart=%s\nreason=%s\ntracking_issue=%s\nexit_criteria=%s\nadded=%s\nadded_by=%s\n",
		chart, entry.Reason, entry.TrackingIssue, entry.ExitCriteria, entry.Added, entry.AddedBy)
	path := filepath.Join(root, "_skip-"+chart+".txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "[SKIPS.yaml] sentinel write %s: %v\n", path, err)
	}
}

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
