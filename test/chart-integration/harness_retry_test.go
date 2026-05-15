//go:build integration

package integration

import (
	"strings"
	"testing"
)

// intPtr is a tiny helper for the LastTerminatedExitCode field.
func intPtr(i int) *int { return &i }

// TestClassifyHelmFailure exercises the pure classifier function
// against the failure modes enumerated in SCR-2026-05-14-001 AC-4.
//
// Naming: `TestClassifyHelmFailure*` so the workflow's existing
// `-run` regex picks them up via the `TestClassifyHelmFailure`
// prefix without editing the comma-separated allowlist (see
// .github/workflows/chart-integration.yaml: the regex already
// matches the prefix `TestClassify...` but NOT this specific
// suite — we extend it in this PR).
func TestClassifyHelmFailure(t *testing.T) {
	cases := []struct {
		name       string
		helmOutput string
		snap       podStatusSnapshot
		want       failureClass
	}{
		{
			name: "ErrImagePull in helm stderr",
			helmOutput: `Error: INSTALLATION FAILED: failed pre-install: 1 error occurred:
	* timed out waiting for the condition
container "main": ErrImagePull
`,
			want: classPull,
		},
		{
			name: "ImagePullBackOff in pod status",
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "p", ContainerStatuses: []containerStatusSnapshot{
					{Name: "c", WaitingReason: "ImagePullBackOff"},
				}},
			}},
			want: classPull,
		},
		{
			name:       "502 Bad Gateway in helm stderr",
			helmOutput: `Error: failed to fetch chart: GET https://ghcr.io/v2/foo/manifests/x.y.z: 502 Bad Gateway`,
			want:       classPull,
		},
		{
			name:       "DNS lookup failure in helm stderr",
			helmOutput: `Error: failed to authorize: failed to fetch oauth token: Get "https://ghcr.io/token": dial tcp: lookup ghcr.io on 10.96.0.10:53: no such host`,
			want:       classPull,
		},
		{
			name:       "TLS handshake timeout to registry",
			helmOutput: `Error: failed to do request: Head ".../manifests/...": net/http: TLS handshake timeout`,
			want:       classPull,
		},
		{
			name:       "manifest unknown from registry",
			helmOutput: `Error: pulling from host ghcr.io failed with status code [manifests v1.0.0]: 404 Not Found: manifest unknown`,
			want:       classPull,
		},
		{
			name: "CrashLoopBackOff in pod status",
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "p", ContainerStatuses: []containerStatusSnapshot{
					{Name: "c", WaitingReason: "CrashLoopBackOff"},
				}},
			}},
			want: classCrash,
		},
		{
			name: "Container terminated with exitCode=1 (crash)",
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "p", ContainerStatuses: []containerStatusSnapshot{
					{Name: "c", LastTerminatedExitCode: intPtr(1)},
				}},
			}},
			want: classCrash,
		},
		{
			name: "CreateContainerConfigError → crash-class",
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "p", ContainerStatuses: []containerStatusSnapshot{
					{Name: "c", WaitingReason: "CreateContainerConfigError"},
				}},
			}},
			want: classCrash,
		},
		{
			name: "RunContainerError → crash-class",
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "p", ContainerStatuses: []containerStatusSnapshot{
					{Name: "c", WaitingReason: "RunContainerError"},
				}},
			}},
			want: classCrash,
		},
		{
			name: "Mixed: helm reports timeout BUT pod is CrashLoopBackOff → crash wins",
			helmOutput: `Error: INSTALLATION FAILED: timed out waiting for the condition
container is in ImagePullBackOff but we should classify by pod truth
`,
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "p", ContainerStatuses: []containerStatusSnapshot{
					// One sidecar pulling, one main container crashing.
					{Name: "sidecar", WaitingReason: "ImagePullBackOff"},
					{Name: "main", WaitingReason: "CrashLoopBackOff"},
				}},
			}},
			want: classCrash,
		},
		{
			name: "Init container crashed → crash-class (init exit nonzero)",
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "p", ContainerStatuses: []containerStatusSnapshot{
					{Name: "init-bootstrap", LastTerminatedExitCode: intPtr(127)},
				}},
			}},
			want: classCrash,
		},
		{
			name: "Container terminated exitCode=0 → not crash (no last terminated indicator)",
			// A graceful exit-0 should NOT be misclassified as crash.
			// Combined with no pull signals, this is unknown.
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "p", ContainerStatuses: []containerStatusSnapshot{
					{Name: "c", LastTerminatedExitCode: intPtr(0)},
				}},
			}},
			want: classUnknown,
		},
		{
			name:       "Empty input → unknown (fail-fast)",
			helmOutput: "",
			snap:       podStatusSnapshot{},
			want:       classUnknown,
		},
		{
			name:       "Generic helm timeout with no pod data → unknown",
			helmOutput: `Error: INSTALLATION FAILED: timed out waiting for the condition`,
			snap:       podStatusSnapshot{},
			want:       classUnknown,
		},
		{
			name:       "Case-insensitive needle match (UPPERCASE 502)",
			helmOutput: `Error: 502 BAD GATEWAY from upstream`,
			want:       classPull,
		},
		{
			name: "Multiple pods, one healthy + one pulling → pull-class",
			snap: podStatusSnapshot{Pods: []podStatus{
				{Name: "ok", ContainerStatuses: []containerStatusSnapshot{{Name: "c"}}},
				{Name: "bad", ContainerStatuses: []containerStatusSnapshot{
					{Name: "c", WaitingReason: "ErrImagePull"},
				}},
			}},
			want: classPull,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyHelmFailure(tc.helmOutput, tc.snap)
			if got != tc.want {
				t.Errorf("classifyHelmFailure() = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestClassifyHelmFailureStringer verifies the failureClass.String()
// outputs match the strings the harness logs (which downstream
// log-scrapers may key on).
func TestClassifyHelmFailureStringer(t *testing.T) {
	cases := map[failureClass]string{
		classUnknown: "unknown",
		classPull:    "pull-class",
		classCrash:   "crash-class",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("failureClass(%d).String() = %q, want %q", c, got, want)
		}
	}
}

// TestClassifyHelmFailureParsesPodJSON exercises parsePodStatusJSON
// against a representative `kubectl get pods -o json` payload,
// ensuring the wire-projection feeds the classifier correctly. This
// closes the loop between the real kubectl output shape and the
// dependency-free classifier input.
func TestClassifyHelmFailureParsesPodJSON(t *testing.T) {
	raw := []byte(`{
		"items": [
			{
				"metadata": {"name": "pod-a", "namespace": "ns"},
				"status": {
					"containerStatuses": [
						{
							"name": "app",
							"state": {"waiting": {"reason": "ImagePullBackOff"}},
							"lastState": {}
						}
					]
				}
			},
			{
				"metadata": {"name": "pod-b", "namespace": "ns"},
				"status": {
					"initContainerStatuses": [
						{
							"name": "init",
							"state": {},
							"lastState": {"terminated": {"exitCode": 2}}
						}
					],
					"containerStatuses": [
						{"name": "main", "state": {}, "lastState": {}}
					]
				}
			}
		]
	}`)
	snap := parsePodStatusJSON(raw)
	if len(snap.Pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(snap.Pods))
	}
	// Sanity-check the projection.
	gotA := snap.Pods[0].ContainerStatuses[0].WaitingReason
	if !strings.EqualFold(gotA, "ImagePullBackOff") {
		t.Errorf("pod-a/app waiting reason = %q, want ImagePullBackOff", gotA)
	}
	// Init container crash should classify the whole thing as crash —
	// this also documents that crash-class wins over pull-class even
	// when the two signals are on DIFFERENT pods, not just different
	// containers within one pod.
	if c := classifyHelmFailure("", snap); c != classCrash {
		t.Errorf("classifier on mixed pull-pod + crashing-init-pod = %s, want crash-class", c)
	}
}

// TestClassifyHelmFailureMalformedJSON verifies parser is robust to
// garbage from kubectl (e.g. a partial dump on connection reset).
func TestClassifyHelmFailureMalformedJSON(t *testing.T) {
	snap := parsePodStatusJSON([]byte(`{garbage`))
	if len(snap.Pods) != 0 {
		t.Errorf("malformed JSON should yield empty snapshot, got %d pods", len(snap.Pods))
	}
	// Empty snapshot + empty helm output → unknown (fail-fast contract).
	if c := classifyHelmFailure("", snap); c != classUnknown {
		t.Errorf("empty everything = %s, want unknown", c)
	}
}
