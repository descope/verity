//go:build integration

package integration

// Pull-vs-crash retry classifier and wrapper for helm install.
//
// Background (SCR-2026-05-14-001, AC-4):
//
//	The chart-integration nightly intermittently fails on transient
//	pull-class errors (ErrImagePull, ImagePullBackOff, registry 5xx,
//	DNS hiccups). These should be retried — they are not bugs in the
//	chart or image.
//
//	Conversely, crash-class errors (CrashLoopBackOff, non-zero exit
//	after the container started) are deterministic chart/image bugs.
//	Retrying them just burns three `helm --wait` timeouts in a row
//	(30+ minutes wasted per chart) without changing the outcome.
//
// This file implements:
//  1. classifyHelmFailure — a *pure* function over
//     (helm combined output, pod status snapshot) that returns
//     one of {classPull, classCrash, classUnknown}. Pure-input
//     so it is exhaustively unit-tested.
//  2. InstallChartWithRetry — wraps InstallChart with up to
//     3 attempts and a 30s backoff, retrying ONLY on classPull.
//     classCrash and classUnknown fail fast.
//
// Helm stderr and `kubectl get pods -o json` are the two evidence
// sources. The classifier prefers pod status over stderr text when
// both are available (e.g. helm prints a generic timeout, but the
// pod is actually crash-looping → crash-class wins).

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/config"
)

// failureClass categorizes a helm install failure for retry decisions.
type failureClass int

const (
	classUnknown failureClass = iota
	classPull
	classCrash
)

func (c failureClass) String() string {
	switch c {
	case classPull:
		return "pull-class"
	case classCrash:
		return "crash-class"
	default:
		return "unknown"
	}
}

// retryConfig holds the wrapper's tunables. Exposed as a struct so
// tests and future callers (e.g. chartValues-aware variants) can
// override without touching the wrapper code.
type retryConfig struct {
	MaxAttempts int
	Backoff     time.Duration
}

func defaultRetryConfig() retryConfig {
	return retryConfig{MaxAttempts: 3, Backoff: 30 * time.Second}
}

// podStatusSnapshot is a minimal, dependency-free projection of the
// fields the classifier needs from `kubectl get pods -o json`. We
// deliberately do NOT import k8s.io/api/core/v1 here — keeping the
// classifier pure and the test surface small.
type podStatusSnapshot struct {
	Pods []podStatus
}

type podStatus struct {
	Name              string
	Namespace         string
	ContainerStatuses []containerStatusSnapshot
}

type containerStatusSnapshot struct {
	Name string
	// WaitingReason is containerStatuses[].state.waiting.reason
	// (empty string if the container is not waiting).
	WaitingReason string
	// LastTerminatedExitCode is lastState.terminated.exitCode
	// (0 if not terminated or terminated cleanly; nonzero = crash).
	// We use *int so we can distinguish "no terminated state" from
	// "terminated with exit 0".
	LastTerminatedExitCode *int
}

// Classifier rule sets. Public-ish (package-level vars) so tests
// can inspect / extend without re-declaring.
var (
	// pullStderrNeedles — substrings in helm combined output that
	// indicate a pull-class failure. Matched case-insensitively.
	pullStderrNeedles = []string{
		"errimagepull",
		"imagepullbackoff",
		"manifest unknown",
		"failed to pull and unpack",
		"failed to resolve reference",
		"connection refused",
		"i/o timeout",
		"no such host",
		"dial tcp",
		"tls handshake timeout",
		"502 bad gateway",
		"503 service unavailable",
		"504 gateway timeout",
	}

	// pullWaitingReasons — pod containerStatuses[].state.waiting.reason
	// values that mean we couldn't fetch the image. Compared exactly
	// (case-insensitive).
	pullWaitingReasons = []string{
		"ErrImagePull",
		"ImagePullBackOff",
		"RegistryUnavailable",
	}

	// crashWaitingReasons — pod waiting reasons that mean the image
	// was pulled successfully but failed to start or stay up. These
	// are deterministic — retry will not help.
	crashWaitingReasons = []string{
		"CrashLoopBackOff",
		"RunContainerError",
		"CreateContainerError",
		"CreateContainerConfigError",
		"StartError",
	}
)

// classifyHelmFailure inspects the helm combined output and the pod
// status snapshot and returns the failure class.
//
// Precedence (intentional):
//  1. crash-class wins over pull-class. If ANY pod is crash-looping,
//     no amount of retrying will help — fail fast even if helm's
//     stderr happens to also mention an image pull (e.g. a sidecar
//     that failed to pull while the main container crashed).
//  2. pull-class is checked second, against both pod waiting reasons
//     and helm stderr needles.
//  3. otherwise → classUnknown (conservative: do not retry).
//
// Empty input → classUnknown.
func classifyHelmFailure(helmOutput string, snap podStatusSnapshot) failureClass {
	// Pass 1: crash-class via pod status (highest priority).
	for _, p := range snap.Pods {
		for _, cs := range p.ContainerStatuses {
			if matchAnyEqualFold(cs.WaitingReason, crashWaitingReasons) {
				return classCrash
			}
			if cs.LastTerminatedExitCode != nil && *cs.LastTerminatedExitCode != 0 {
				// Container started, then exited non-zero. This is
				// a crash regardless of whether kubelet has yet
				// flipped the waiting reason to CrashLoopBackOff.
				return classCrash
			}
		}
	}

	// Pass 2: pull-class via pod status.
	for _, p := range snap.Pods {
		for _, cs := range p.ContainerStatuses {
			if matchAnyEqualFold(cs.WaitingReason, pullWaitingReasons) {
				return classPull
			}
		}
	}

	// Pass 3: pull-class via helm stderr needles. Lowercase once.
	lower := strings.ToLower(helmOutput)
	for _, n := range pullStderrNeedles {
		if strings.Contains(lower, n) {
			return classPull
		}
	}

	return classUnknown
}

func matchAnyEqualFold(s string, set []string) bool {
	if s == "" {
		return false
	}
	for _, candidate := range set {
		if strings.EqualFold(s, candidate) {
			return true
		}
	}
	return false
}

// InstallChartWithRetry wraps InstallChart. It performs up to
// cfg.MaxAttempts attempts with cfg.Backoff between failures, but
// only retries when the classifier returns classPull. Crash-class
// and unknown-class failures abort immediately.
//
// Between retries, the namespace is torn down via UninstallChart so
// the next attempt starts clean (re-uses the existing teardown path
// — no bespoke cleanup logic).
//
// On final failure, returns the ChartContext from the last attempt
// (so callers can still dump diagnostics) and the last underlying
// error wrapped with the attempt count and classification.
func InstallChartWithRetry(
	ctx context.Context,
	h *Harness,
	spec config.ChartSpec,
	valuesDir string,
	cfg retryConfig,
) (*ChartContext, error) {
	return installChartWithRetryInNamespace(ctx, h, spec, valuesDir, sanitizeNamespace(spec.Name), preserveNamespaceOnRetry(spec.Name), cfg)
}

func installChartWithRetryInNamespace(
	ctx context.Context,
	h *Harness,
	spec config.ChartSpec,
	valuesDir string,
	namespace string,
	preserveNamespace bool,
	cfg retryConfig,
) (*ChartContext, error) {
	if cfg.MaxAttempts <= 0 {
		cfg = defaultRetryConfig()
	}
	var lastCC *ChartContext
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		cc, err := installChartInNamespace(ctx, h, spec, valuesDir, namespace, preserveNamespace)
		if err == nil {
			if attempt > 1 {
				h.t.Logf("chart-integration[%s]: install succeeded on attempt %d/%d", spec.Name, attempt, cfg.MaxAttempts)
			}
			return cc, nil
		}
		lastCC = cc
		lastErr = err

		// Snapshot pod status from the failing namespace (best-effort).
		var snap podStatusSnapshot
		if cc != nil && h.KubeconfigPath != "" {
			snap = gatherPodStatuses(ctx, h, cc.Namespace)
		}
		class := classifyHelmFailure(err.Error(), snap)

		isLast := attempt == cfg.MaxAttempts
		switch {
		case class == classPull && !isLast:
			h.t.Logf("chart-integration[%s]: attempt %d/%d failed: classification=%s, retrying in %s",
				spec.Name, attempt, cfg.MaxAttempts, class, cfg.Backoff)
			// Best-effort cleanup before next attempt — reuse the
			// existing teardown path so we don't reimplement it.
			//
			// UninstallChart uses `kubectl delete ns --wait=false` so
			// the final teardown after runChart returns is fast. For
			// the retry path that semantics is wrong: starting a
			// fresh `helm install` against a namespace still in
			// Terminating state fails with `namespace … is being
			// terminated` or AlreadyExists, which the classifier
			// would treat as an unrelated failure and abort the
			// retry budget.
			//
			// Block here until the namespace is fully deleted (or
			// the wait budget expires); only then start the backoff
			// timer. The 90s budget accommodates kind's local-path
			// PVC reclaim plus finalizer drain for stateful charts.
			if cc != nil {
				uctx, ucancel := context.WithTimeout(ctx, 3*time.Minute)
				UninstallChart(uctx, h, cc)
				ucancel()
				wctx, wcancel := context.WithTimeout(ctx, 90*time.Second)
				if err := waitNamespaceDeleted(wctx, h, cc.Namespace); err != nil {
					h.t.Logf("chart-integration[%s]: attempt %d cleanup: namespace %q deletion did not complete within budget: %v (continuing)",
						spec.Name, attempt, cc.Namespace, err)
				}
				wcancel()
			}
			select {
			case <-ctx.Done():
				return lastCC, fmt.Errorf("install (after %d attempts, last classification=%s): %w: context cancelled during backoff",
					attempt, class, lastErr)
			case <-time.After(cfg.Backoff):
			}
		default:
			// Crash-class, unknown, or final attempt — fail fast.
			h.t.Logf("chart-integration[%s]: attempt %d/%d failed: classification=%s, NOT retrying",
				spec.Name, attempt, cfg.MaxAttempts, class)
			return lastCC, fmt.Errorf("install (attempt %d/%d, classification=%s): %w",
				attempt, cfg.MaxAttempts, class, err)
		}
	}
	// Defensive — shouldn't be reachable given loop logic above.
	return lastCC, fmt.Errorf("install: exhausted %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// gatherPodStatuses snapshots `kubectl get pods -o json -n <ns>` and
// projects it into the dependency-free podStatusSnapshot shape used
// by the classifier. Errors are swallowed (best-effort) — the
// classifier is robust to an empty snapshot.
func gatherPodStatuses(ctx context.Context, h *Harness, namespace string) podStatusSnapshot {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "kubectl",
		"--kubeconfig", h.KubeconfigPath,
		"-n", namespace,
		"get", "pods", "-o", "json",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return podStatusSnapshot{}
	}
	return parsePodStatusJSON(out)
}

// parsePodStatusJSON unmarshals the subset of `kubectl get pods -o json`
// we care about. Separated from gatherPodStatuses so the parser is
// unit-testable without exec'ing kubectl.
func parsePodStatusJSON(raw []byte) podStatusSnapshot {
	// Minimal projection of the k8s PodList shape — only the fields
	// the classifier reads. Anything else is ignored by encoding/json.
	type wireContainerState struct {
		Waiting *struct {
			Reason string `json:"reason"`
		} `json:"waiting"`
		Terminated *struct {
			ExitCode int `json:"exitCode"`
		} `json:"terminated"`
	}
	type wireContainerStatus struct {
		Name      string             `json:"name"`
		State     wireContainerState `json:"state"`
		LastState wireContainerState `json:"lastState"`
	}
	type wirePod struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Status struct {
			ContainerStatuses     []wireContainerStatus `json:"containerStatuses"`
			InitContainerStatuses []wireContainerStatus `json:"initContainerStatuses"`
		} `json:"status"`
	}
	type wireList struct {
		Items []wirePod `json:"items"`
	}
	var list wireList
	if err := json.Unmarshal(raw, &list); err != nil {
		return podStatusSnapshot{}
	}
	snap := podStatusSnapshot{Pods: make([]podStatus, 0, len(list.Items))}
	for _, p := range list.Items {
		ps := podStatus{Name: p.Metadata.Name, Namespace: p.Metadata.Namespace}
		// Merge init + regular container statuses — failures in
		// either category should be classified.
		all := append([]wireContainerStatus{}, p.Status.InitContainerStatuses...)
		all = append(all, p.Status.ContainerStatuses...)
		for _, cs := range all {
			snapCS := containerStatusSnapshot{Name: cs.Name}
			if cs.State.Waiting != nil {
				snapCS.WaitingReason = cs.State.Waiting.Reason
			}
			if cs.LastState.Terminated != nil {
				code := cs.LastState.Terminated.ExitCode
				snapCS.LastTerminatedExitCode = &code
			}
			ps.ContainerStatuses = append(ps.ContainerStatuses, snapCS)
		}
		snap.Pods = append(snap.Pods, ps)
	}
	return snap
}

// waitNamespaceDeleted blocks until `kubectl get namespace <ns>` returns
// NotFound, or the supplied context expires. Used by the retry path to
// ensure a Terminating namespace from a previous attempt fully drains
// (finalizers, PVC reclaim, helm release Secret removal) before the
// next `helm install` runs, otherwise the next install would race
// the still-terminating namespace and fail with
//
//	`namespace "<ns>" is being terminated`
//
// or AlreadyExists, neither of which is a pull-class failure and so
// would abort the retry budget on a transient teardown lag.
//
// Errors are returned to the caller for logging only — the retry
// loop continues into the backoff even on wait-budget exhaustion, so
// the worst case is a redundant attempt that fails fast.
func waitNamespaceDeleted(ctx context.Context, h *Harness, namespace string) error {
	if namespace == "" {
		return nil
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	// Probe once immediately so the common case (already deleted by
	// the time we get here) returns without a 2s delay.
	if missing, _ := namespaceIsGone(ctx, h, namespace); missing {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for namespace %q deletion: %w", namespace, ctx.Err())
		case <-ticker.C:
			missing, err := namespaceIsGone(ctx, h, namespace)
			if err != nil {
				return fmt.Errorf("waiting for namespace %q deletion: %w", namespace, err)
			}
			if missing {
				return nil
			}
		}
	}
}

// namespaceIsGone returns (true, nil) when `kubectl get namespace`
// reports NotFound. Any other error (including transient connection
// failures) is returned as-is so the caller can decide whether to
// keep polling. A present-but-Terminating namespace returns
// (false, nil).
func namespaceIsGone(ctx context.Context, h *Harness, namespace string) (bool, error) {
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", h.KubeconfigPath,
		"get", "namespace", namespace,
		"--ignore-not-found",
		"--output=jsonpath={.metadata.name}",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// kubectl exits non-zero on connection errors etc. Surface
		// the underlying message so retry-loop callers can log it.
		return false, fmt.Errorf("kubectl get namespace %s: %s: %w",
			namespace, strings.TrimSpace(string(out)), err)
	}
	// --ignore-not-found + jsonpath=.metadata.name returns empty
	// when the namespace doesn't exist, and the namespace name
	// when it does (whether or not it's Terminating).
	return strings.TrimSpace(string(out)) == "", nil
}
