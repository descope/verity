//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

func TestLoadAllowlist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alist.txt")
	body := "" +
		"# header comment\n" +
		"\n" +
		"docker.io/library/something\n" +
		"  quay.io/special/image  \n" +
		"# trailing comment\n" +
		"registry.k8s.io/foo\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadAllowlist(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker.io/library/something",
		"quay.io/special/image",
		"registry.k8s.io/foo",
	}
	if len(got) != len(want) {
		t.Fatalf("len got=%d want=%d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx=%d got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestLoadAllowlistMissing(t *testing.T) {
	// SCR-2026-04-30-001 (Option E): missing allowlist files are a
	// typed signal, not a silent empty allowlist. Callers must be
	// able to branch on errAllowlistMissing via errors.Is. The
	// returned slice is nil so the previous "(nil, nil)" callers
	// observe the same slice shape, just with a non-nil error.
	missingPath := filepath.Join(t.TempDir(), "does-not-exist.txt")
	got, err := loadAllowlist(missingPath)
	if err == nil {
		t.Fatalf("missing file must return errAllowlistMissing, got nil error (slice=%v)", got)
	}
	if !errors.Is(err, errAllowlistMissing) {
		t.Fatalf("missing file must return errAllowlistMissing (errors.Is), got %T: %v", err, err)
	}
	if got != nil {
		t.Fatalf("expected nil slice on missing file, got %v", got)
	}
	// The wrapped error must mention the requested path so
	// diagnostics in the test driver can name the missing file.
	if !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("error message must contain requested path %q, got %q", missingPath, err.Error())
	}
}

func TestLoadAllowlistPresentRegression(t *testing.T) {
	// Regression: loadAllowlist still parses an existing file
	// correctly after the missing-file branch was promoted to a
	// typed error. Mirrors TestLoadAllowlist with a smaller body
	// to give the assertion a second, narrower data point.
	dir := t.TempDir()
	path := filepath.Join(dir, "alist.txt")
	body := "" +
		"# comment\n" +
		"\n" +
		"  registry.k8s.io/sig-storage/csi-provisioner  \n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadAllowlist(path)
	if err != nil {
		t.Fatalf("present file must not error: %v", err)
	}
	if errors.Is(err, errAllowlistMissing) {
		t.Fatalf("present file must NOT return errAllowlistMissing")
	}
	if len(got) != 1 || got[0] != "registry.k8s.io/sig-storage/csi-provisioner" {
		t.Fatalf("got %v, want exactly one trimmed entry", got)
	}
}

func TestClassifyMissingAllowlist(t *testing.T) {
	// SCR-2026-04-30-001 (Option E) policy decision table: the
	// driver hard-fails iff at least one observed image is non-
	// verity-prefixed. classifyMissingAllowlist returns the
	// offenders; an empty return = clean pass.
	cases := []struct {
		name      string
		images    []string
		offenders []string
	}{
		{
			name:      "missing+all-verity (clean pass)",
			images:    []string{"ghcr.io/verity-org/prometheus/prometheus:v3.9.1", "ghcr.io/verity-org/library/nginx:1.29.5-patched"},
			offenders: nil,
		},
		{
			name:      "missing+non-verity (hard fail)",
			images:    []string{"ghcr.io/verity-org/prometheus/prometheus:v3.9.1", "quay.io/upstream/foo:latest"},
			offenders: []string{"quay.io/upstream/foo:latest"},
		},
		{
			name:      "missing+empty namespace (clean pass — no images, no policy)",
			images:    nil,
			offenders: nil,
		},
		{
			name:      "missing+all-non-verity (hard fail, every image flagged)",
			images:    []string{"docker.io/library/postgres:17", "registry.k8s.io/sig-storage/csi-provisioner:v5.0.0"},
			offenders: []string{"docker.io/library/postgres:17", "registry.k8s.io/sig-storage/csi-provisioner:v5.0.0"},
		},
		{
			name:      "missing+double-ghcr-bug-pattern (chart-gen rewrite gap is non-verity)",
			images:    []string{"ghcr.io/ghcr.io/verity-org/foo:tag"},
			offenders: []string{"ghcr.io/ghcr.io/verity-org/foo:tag"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyMissingAllowlist(c.images)
			if len(got) != len(c.offenders) {
				t.Fatalf("len got=%d want=%d (got=%v want=%v)", len(got), len(c.offenders), got, c.offenders)
			}
			for i := range c.offenders {
				if got[i] != c.offenders[i] {
					t.Fatalf("idx=%d got=%q want=%q", i, got[i], c.offenders[i])
				}
			}
		})
	}
}

// TestClassifyMissingAllowlistPresentMixed documents the policy
// boundary: classifyMissingAllowlist is only used when the
// allowlist file is *absent*. When present, AssertImageOrigin
// runs unchanged through isAccepted with a populated allow slice
// (covered by TestIsAccepted). This test asserts the helper's
// correct posture: it does NOT consult any allowlist, so a
// "present+mixed" caller would never reach it. We document that
// invariant here as a guard against future drift.
func TestClassifyMissingAllowlistDoesNotConsultAllowlist(t *testing.T) {
	// Even an image that WOULD be allowlisted under a
	// hypothetical present-allowlist test is still flagged here,
	// because classifyMissingAllowlist's whole point is "no
	// allowlist file exists, so non-verity images have no path
	// to acceptance."
	images := []string{"quay.io/special/escape-hatch:latest"}
	got := classifyMissingAllowlist(images)
	if len(got) != 1 || got[0] != images[0] {
		t.Fatalf("classifyMissingAllowlist must not consult an implicit allowlist; got %v", got)
	}
}

func TestIsAccepted(t *testing.T) {
	allow := []string{
		"quay.io/special/escape-hatch",
		"registry.k8s.io/known-cant-rewrite",
		"registry.k8s.io/ingress-nginx/controller@",
	}
	cases := []struct {
		name    string
		image   string
		imageID string
		want    bool
	}{
		{"verity registry", "ghcr.io/verity-org/prometheus/prometheus:v3.9.1", "", true},
		{"verity registry verbose", "ghcr.io/verity-org/library/nginx:1.29.5-patched", "", true},
		{"allowlist exact", "quay.io/special/escape-hatch:latest", "", true},
		{"allowlist prefix", "registry.k8s.io/known-cant-rewrite/sub:v1", "", true},
		{"upstream rejected", "quay.io/prometheus/prometheus:v3.9.1", "", false},
		{"docker hub rejected", "docker.io/library/postgres:17", "", false},
		{"empty rejected", "", "", false},
		{"double-ghcr (chart-gen bug pattern) rejected", "ghcr.io/ghcr.io/verity-org/foo:tag", "", false},
		// Bare-digest fallback: kubelet sometimes records the
		// container status image as just "sha256:<hex>" (no registry
		// path), with the canonical reference preserved in imageID.
		// isAccepted MUST fall back to imageID for allowlist matching
		// in that case. See SCR-2026-04-30-001 + assertions.go
		// containerStatus.Image docstring for the underlying cause.
		{
			name:    "bare digest with allowlisted imageID accepted",
			image:   "sha256:895ddb49053a9b80e1c97354a933f59cc94fba4b6f831615687151c9b178218d",
			imageID: "registry.k8s.io/ingress-nginx/controller@sha256:594ceea76b01c592858f803f9ff4d2cb40542cae2060410b2c95f75907d659e1",
			want:    true,
		},
		{
			name:    "bare digest with verity-prefixed imageID accepted",
			image:   "sha256:deadbeef",
			imageID: "ghcr.io/verity-org/library/foo@sha256:deadbeef",
			want:    true,
		},
		{
			name:    "bare digest with non-allowlisted imageID rejected",
			image:   "sha256:cafebabe",
			imageID: "docker.io/library/postgres@sha256:cafebabe",
			want:    false,
		},
		{
			name:    "bare digest with empty imageID rejected (no fallback target)",
			image:   "sha256:cafebabe",
			imageID: "",
			want:    false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isAccepted(c.image, c.imageID, allow)
			if got != c.want {
				t.Fatalf("isAccepted(image=%q, imageID=%q) = %v, want %v", c.image, c.imageID, got, c.want)
			}
		})
	}
}

func TestHarnessClusterCreatedFieldDefault(t *testing.T) {
	h := NewHarness(stdoutLogger{}, "/tmp")
	if h.clusterCreated {
		t.Fatal("new Harness must start with clusterCreated=false; otherwise Teardown could delete a pre-existing cluster the harness did not create")
	}
}

// makePodForRestartTest builds a single-pod podList for
// TestCollectRestartFailures. Centralised here so each table-driven
// case is a flat literal — keeps cyclomatic complexity / maintidx
// low in the test function itself.
func makePodForRestartTest(namespace, name string, main, init []containerStatus) *podList {
	return makePodWithOwnerForRestartTest(namespace, name, "", "", main, init)
}

// makePodWithOwnerForRestartTest builds a single-pod podList with
// optional ownerReferences[0].kind + status.phase. Used by the
// Job-owned-pod main-container exemption tests; passing empty
// strings reduces to the default makePodForRestartTest shape.
func makePodWithOwnerForRestartTest(namespace, name, ownerKind, phase string, main, init []containerStatus) *podList {
	var owners []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if ownerKind != "" {
		owners = []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		}{{Kind: ownerKind, Name: name + "-owner"}}
	}
	return &podList{Items: []struct {
		Metadata struct {
			Name            string `json:"name"`
			Namespace       string `json:"namespace"`
			OwnerReferences []struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
		Status struct {
			Phase                 string            `json:"phase"`
			ContainerStatuses     []containerStatus `json:"containerStatuses"`
			InitContainerStatuses []containerStatus `json:"initContainerStatuses"`
		} `json:"status"`
	}{{
		Metadata: struct {
			Name            string `json:"name"`
			Namespace       string `json:"namespace"`
			OwnerReferences []struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"ownerReferences"`
		}{Name: name, Namespace: namespace, OwnerReferences: owners},
		Status: struct {
			Phase                 string            `json:"phase"`
			ContainerStatuses     []containerStatus `json:"containerStatuses"`
			InitContainerStatuses []containerStatus `json:"initContainerStatuses"`
		}{Phase: phase, ContainerStatuses: main, InitContainerStatuses: init},
	}}}
}

func mainCS(name string, restarts int, lastTermReason, lastTermMsg string) containerStatus {
	var cs containerStatus
	cs.Name = name
	cs.RestartCount = restarts
	if lastTermReason != "" || lastTermMsg != "" {
		cs.LastTerminationState.Terminated = &struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}{Reason: lastTermReason, Message: lastTermMsg}
	}
	return cs
}

func initCSCompleted(name string, restarts, exitCode int) containerStatus {
	cs := mainCS(name, restarts, "Error", "first attempt failed")
	cs.State.Terminated = &struct {
		Reason   string `json:"reason"`
		Message  string `json:"message"`
		ExitCode int    `json:"exitCode"`
	}{Reason: "Completed", ExitCode: exitCode}
	return cs
}

func initCSWaiting(name string, restarts int, waitingReason string) containerStatus {
	cs := mainCS(name, restarts, "Error", "exit code 1")
	cs.State.Waiting = &struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}{Reason: waitingReason}
	return cs
}

// TestCollectRestartFailures pins the no-restart gate policy:
//
//   - Main containers: ANY restartCount>0 is a hard failure
//     (CrashLoopBackOff / OOM during settle window — the gate's
//     entire purpose).
//   - InitContainers: restartCount>0 is exempted iff the init
//     ultimately completed cleanly (state.terminated.exitCode==0).
//     Charts like crossplane race the kube-apiserver during their
//     init CRD wait and occasionally need one restart to succeed;
//     the cluster is healthy in that case and the gate must not
//     fire. A crash-looping init (waiting / non-zero terminated /
//     still running) is NOT exempted — the gate fires as before.
func TestCollectRestartFailures(t *testing.T) {
	cases := []struct {
		name            string
		pods            *podList
		wantFailCount   int
		wantSubstrings  []string
		bannedSubstring string
	}{
		{
			name:          "main container with no restarts: clean",
			pods:          makePodForRestartTest("ns", "pod-a", []containerStatus{{Name: "app", RestartCount: 0}}, nil),
			wantFailCount: 0,
		},
		{
			name:           "main container with restart: fails",
			pods:           makePodForRestartTest("ns", "pod-b", []containerStatus{mainCS("app", 2, "OOMKilled", "out of memory")}, nil),
			wantFailCount:  1,
			wantSubstrings: []string{"container=app", "restarts=2", "OOMKilled"},
		},
		{
			// Reproduces the crossplane case: init container
			// restarted once due to a CRD-availability race, then
			// completed cleanly. Main container is now running.
			// Gate must NOT fire — the cluster is healthy.
			name: "initContainer restarted but ultimately succeeded (exitCode 0): exempted",
			pods: makePodForRestartTest("crossplane", "crossplane-xxx",
				[]containerStatus{{Name: "crossplane", RestartCount: 0}},
				[]containerStatus{initCSCompleted("crossplane-init", 1, 0)}),
			wantFailCount: 0,
		},
		{
			// Real crash loop in an init container is still a gate
			// failure: the init has not succeeded, so the workload
			// is not actually up. Gate MUST fire.
			name:           "initContainer crash-looping (waiting CrashLoopBackOff): fails",
			pods:           makePodForRestartTest("ns", "app-xxx", nil, []containerStatus{initCSWaiting("wait-for-db", 3, "CrashLoopBackOff")}),
			wantFailCount:  1,
			wantSubstrings: []string{"initContainer=wait-for-db", "restarts=3"},
		},
		{
			// Final exit code non-zero means init truly failed
			// (pod would be stuck). Gate MUST fire — exemption is
			// narrow: exitCode==0 only.
			name:          "initContainer non-zero terminated despite restartCount: fails",
			pods:          makePodForRestartTest("ns", "app-yyy", nil, []containerStatus{initCSCompleted("migrate", 1, 1)}),
			wantFailCount: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := collectRestartFailures(c.pods)
			if len(got) != c.wantFailCount {
				t.Fatalf("got %d failures, want %d: %v", len(got), c.wantFailCount, got)
			}
			if c.wantFailCount == 0 {
				return
			}
			for _, sub := range c.wantSubstrings {
				if !strings.Contains(got[0], sub) {
					t.Fatalf("failure %q missing substring %q", got[0], sub)
				}
			}
		})
	}
}

// TestCollectRestartFailures_MultiPodMix asserts the gate's
// per-pod independence: a benign init flake on one pod and a real
// crash on another pod are reported correctly as exactly one
// failure attributed to the crashing pod.
func TestCollectRestartFailures_MultiPodMix(t *testing.T) {
	initOK := initCSCompleted("init", 1, 0)
	mainBad := mainCS("worker", 5, "OOMKilled", "")

	pods := &podList{Items: []struct {
		Metadata struct {
			Name            string `json:"name"`
			Namespace       string `json:"namespace"`
			OwnerReferences []struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
		Status struct {
			Phase                 string            `json:"phase"`
			ContainerStatuses     []containerStatus `json:"containerStatuses"`
			InitContainerStatuses []containerStatus `json:"initContainerStatuses"`
		} `json:"status"`
	}{
		// pod "good": clean main + exempted init flake
		makePodForRestartTest("ns", "good", []containerStatus{{Name: "app", RestartCount: 0}}, []containerStatus{initOK}).Items[0],
		// pod "bad": main crash-looping
		makePodForRestartTest("ns", "bad", []containerStatus{mainBad}, nil).Items[0],
	}}
	got := collectRestartFailures(pods)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 failure (only the OOMKilled main container); got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "pod=ns/bad") || !strings.Contains(got[0], "container=worker") {
		t.Fatalf("wrong failure attributed: %q", got[0])
	}
}

// jobCompletedMainCS builds a main containerStatus that mimics the
// terminal state of a Job pod whose first container attempt failed
// and second attempt succeeded: restartCount>0, current state is
// terminated with exitCode 0.
func jobCompletedMainCS(name string, restarts int) containerStatus {
	cs := mainCS(name, restarts, "Error", "first attempt: connection refused")
	cs.State.Terminated = &struct {
		Reason   string `json:"reason"`
		Message  string `json:"message"`
		ExitCode int    `json:"exitCode"`
	}{Reason: "Completed", ExitCode: 0}
	return cs
}

// TestCollectRestartFailures_JobOwnedPodExemption pins the
// Job-owned-pod main-container exemption: a one-shot Job pod whose
// first container attempt failed and second attempt succeeded
// (final exitCode 0, pod phase Succeeded) MUST NOT trip the
// no-restart gate. The gate's purpose is to catch CrashLoopBackOff
// / OOMKill on long-running workloads, not benign Job retries that
// ultimately completed cleanly.
//
// Negative cases (Job pod where the final state is still Failed,
// or non-Job pod with the same restart pattern) still trip the
// gate — the exemption is narrow: kind=Job AND phase=Succeeded AND
// terminated{exitCode:0}.
func TestCollectRestartFailures_JobOwnedPodExemption(t *testing.T) {
	cases := []struct {
		name          string
		ownerKind     string
		phase         string
		main          []containerStatus
		wantFailCount int
	}{
		{
			// The airflow run-airflow-migrations case: Job pod,
			// first attempt failed (transient DB unavailability),
			// second attempt succeeded, phase Succeeded.
			name:          "Job-owned pod main container retried then succeeded: exempted",
			ownerKind:     "Job",
			phase:         "Succeeded",
			main:          []containerStatus{jobCompletedMainCS("run-airflow-migrations", 1)},
			wantFailCount: 0,
		},
		{
			// Deployment-owned pod with the same restart pattern
			// is NOT exempted — long-running workloads should not
			// have any restarts during settle window.
			name:          "Deployment-owned pod main container retried then succeeded: NOT exempted",
			ownerKind:     "ReplicaSet",
			phase:         "Running",
			main:          []containerStatus{jobCompletedMainCS("api-server", 1)},
			wantFailCount: 1,
		},
		{
			// Job-owned pod still running / pod phase Failed is
			// NOT exempted — the exemption is narrow: the Job
			// must have reached Succeeded.
			name:          "Job-owned pod main container crash-looping (phase Failed): NOT exempted",
			ownerKind:     "Job",
			phase:         "Failed",
			main:          []containerStatus{mainCS("flaky", 3, "Error", "exit code 1")},
			wantFailCount: 1,
		},
		{
			// Job-owned pod, phase Succeeded, but the container's
			// final state is non-zero exit — should NOT happen in
			// practice (k8s would mark the pod Failed) but pin the
			// narrowness of the exemption: terminated{exitCode:0}
			// is REQUIRED.
			name:      "Job-owned pod phase Succeeded but final exit non-zero: NOT exempted",
			ownerKind: "Job",
			phase:     "Succeeded",
			main: []containerStatus{func() containerStatus {
				cs := mainCS("weird", 1, "Error", "")
				cs.State.Terminated = &struct {
					Reason   string `json:"reason"`
					Message  string `json:"message"`
					ExitCode int    `json:"exitCode"`
				}{Reason: "Error", ExitCode: 1}
				return cs
			}()},
			wantFailCount: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pods := makePodWithOwnerForRestartTest("ns", "pod-x", c.ownerKind, c.phase, c.main, nil)
			got := collectRestartFailures(pods)
			if len(got) != c.wantFailCount {
				t.Fatalf("got %d failures, want %d: %v", len(got), c.wantFailCount, got)
			}
		})
	}
}

// TestIsOwnedByJob pins the predicate: only an ownerReference of
// kind "Job" returns true; everything else (ReplicaSet, StatefulSet,
// CronJob, nil) returns false. CronJob does NOT pass because pods
// created by a CronJob run have a direct `Kind: Job` owner — the
// CronJob is one level removed from the pod.
func TestIsOwnedByJob(t *testing.T) {
	mk := func(kinds ...string) []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} {
		out := make([]struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		}, 0, len(kinds))
		for _, k := range kinds {
			out = append(out, struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			}{Kind: k})
		}
		return out
	}
	cases := []struct {
		name string
		refs []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		}
		want bool
	}{
		{"empty owners", nil, false},
		{"single Job owner", mk("Job"), true},
		{"single ReplicaSet owner", mk("ReplicaSet"), false},
		{"CronJob owner (does NOT match — pods have direct Job owner)", mk("CronJob"), false},
		{"multiple owners, includes Job", mk("StatefulSet", "Job"), true},
		{"multiple owners, no Job", mk("StatefulSet", "ReplicaSet"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isOwnedByJob(c.refs); got != c.want {
				t.Fatalf("isOwnedByJob(%v) = %v, want %v", c.refs, got, c.want)
			}
		})
	}
}

// TestInitContainerCompletedCleanly pins the predicate's narrow
// semantics: ONLY terminated{exitCode:0} counts as a clean
// completion. Anything else (still running, waiting, failed
// terminated) returns false so the no-restart gate stays armed
// against real init crash loops.
func TestInitContainerCompletedCleanly(t *testing.T) {
	mkTerm := func(code int) containerStatus {
		var cs containerStatus
		cs.State.Terminated = &struct {
			Reason   string `json:"reason"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		}{ExitCode: code}
		return cs
	}
	mkWait := func(reason string) containerStatus {
		var cs containerStatus
		cs.State.Waiting = &struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}{Reason: reason}
		return cs
	}

	cases := []struct {
		name string
		cs   containerStatus
		want bool
	}{
		{"terminated exitCode 0", mkTerm(0), true},
		{"terminated exitCode 1", mkTerm(1), false},
		{"terminated exitCode 137 (SIGKILL)", mkTerm(137), false},
		{"waiting CrashLoopBackOff", mkWait("CrashLoopBackOff"), false},
		{"waiting PodInitializing", mkWait("PodInitializing"), false},
		{"empty state (running, no fields populated)", containerStatus{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cs := c.cs
			if got := initContainerCompletedCleanly(&cs); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestInstallChartRejectsArgumentInjection(t *testing.T) {
	cases := []struct {
		name string
		spec config.ChartSpec
	}{
		{"name starts with dash", config.ChartSpec{Name: "-rf", Version: "1.0.0", Repository: "oci://example.com/charts"}},
		{"version starts with dash", config.ChartSpec{Name: "ok", Version: "--exec", Repository: "oci://example.com/charts"}},
		{"non-oci/http repo", config.ChartSpec{Name: "ok", Version: "1.0.0", Repository: "ssh://example.com/charts"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := InstallChart(context.Background(), &Harness{t: stdoutLogger{}}, c.spec, "")
			if err == nil {
				t.Fatalf("InstallChart should reject %v as argument-injection risk", c.spec)
			}
		})
	}
}

func TestOpenSearchDashboardsInstallsOpenSearchPrerequisiteInSameNamespace(t *testing.T) {
	prereqs := chartPrerequisites["opensearch-dashboards"]
	if len(prereqs) != 1 {
		t.Fatalf("opensearch-dashboards prereqs = %#v, want exactly one", prereqs)
	}
	if prereqs[0].Name != "opensearch" {
		t.Fatalf("prereq name = %q, want opensearch", prereqs[0].Name)
	}
	if !prereqs[0].SameNamespace {
		t.Fatalf("opensearch prerequisite must install in the dashboard namespace so opensearch-cluster-master resolves")
	}
}
