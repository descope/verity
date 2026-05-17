//go:build integration

package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	settleDuration       = 30 * time.Second
	verityRegistryPrefix = "ghcr.io/verity-org/"
)

var (
	errContainerRestarted = errors.New("container restarted during settle window")
	errImageNotAccepted   = errors.New("container image not from ghcr.io/verity-org and not allowlisted")
	// errAllowlistMissing is returned by loadAllowlist when the
	// requested allowlist file does not exist on disk. Per
	// SCR-2026-04-30-001 (Option E), missing-allowlist is a typed
	// signal — not a silent empty allowlist — so the test driver
	// can decide policy: hard-fail when any non-verity image is
	// observed in the namespace, pass cleanly when every image is
	// already verity-prefixed (the verityRegistryPrefix
	// short-circuit in isAccepted suffices). Tests must consume
	// this via errors.Is.
	errAllowlistMissing = errors.New("allowlist file not found")
)

type podList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			// OwnerReferences lets the no-restart gate distinguish
			// Job-owned pods (one-shot workloads that legitimately
			// rerun a failed container in-place when restartPolicy
			// is OnFailure) from Deployment / StatefulSet pods
			// (long-running workloads where any restart is a hard
			// signal). See collectRestartFailures for the
			// Job-owned-pod exemption rationale.
			OwnerReferences []struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
		Status struct {
			// Phase is the pod's lifecycle phase. For Job-owned
			// pods we accept Succeeded as the terminal state when
			// admitting a Job-pod main-container-restart exemption
			// (the Job ran, the container retried, the final
			// attempt exited 0, k8s marked the pod Succeeded).
			Phase                 string            `json:"phase"`
			ContainerStatuses     []containerStatus `json:"containerStatuses"`
			InitContainerStatuses []containerStatus `json:"initContainerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

type containerStatus struct {
	Name string `json:"name"`
	// Image is the container image string as reported by kubelet in
	// .status.containerStatuses[].image. For most pods this is the
	// same registry-prefixed reference present in the pod spec
	// (e.g. "registry.k8s.io/ingress-nginx/controller:v1.15.1").
	//
	// HOWEVER, when a pod's spec.image carries BOTH a tag and a
	// digest pin (e.g. "...controller:v1.15.1@sha256:594ce..."), the
	// containerd/CRI implementation under kind sometimes normalises
	// the rendered status.image down to the bare local digest
	// ("sha256:895dd..."), discarding the registry path entirely.
	// In that case ImageID still carries the canonical registry-
	// prefixed digest reference (e.g.
	// "registry.k8s.io/ingress-nginx/controller@sha256:594ce..."),
	// so allowlist matching MUST fall back to ImageID when Image is
	// a bare digest. See isAccepted for the fallback logic and
	// SCR-2026-04-30-001 for the policy rationale.
	Image        string `json:"image"`
	ImageID      string `json:"imageID"`
	RestartCount int    `json:"restartCount"`
	// State is the container's CURRENT state — running / waiting /
	// terminated. For initContainers we use this to distinguish
	// "init flaked once but ultimately completed (exitCode 0)" from
	// "init is still crash-looping". Only the latter trips the
	// no-restart gate; the former is benign (the main container is
	// already past the init phase and running). See
	// collectRestartFailures.
	State struct {
		Terminated *struct {
			Reason   string `json:"reason"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		} `json:"terminated"`
		Waiting *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"waiting"`
	} `json:"state"`
	LastTerminationState struct {
		Terminated *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"terminated"`
	} `json:"lastState"`
}

func WaitSettle(ctx context.Context) {
	t := time.NewTimer(settleDuration)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func AssertNoRestarts(ctx context.Context, h *Harness, namespace string) error {
	pods, err := getPods(ctx, h, namespace)
	if err != nil {
		return err
	}
	failures := collectRestartFailures(pods)
	if len(failures) > 0 {
		return fmt.Errorf("%w: count=%d namespace=%s\n  %s",
			errContainerRestarted, len(failures), namespace, strings.Join(failures, "\n  "))
	}
	return nil
}

func collectRestartFailures(pods *podList) []string {
	var failures []string
	for _, p := range pods.Items {
		// Job-owned pods are one-shot workloads. A Job with the
		// default `restartPolicy: OnFailure` retries the container
		// in-place when its first attempt exits non-zero (transient
		// DB unavailability while the postgres subchart is still
		// starting is a common cause). The final attempt exits 0,
		// the pod transitions to Succeeded, and the Job is
		// Complete — that's the success state. The no-restart gate
		// is meant to catch CrashLoopBackOff / OOMKill on
		// long-running workload containers, NOT a benign Job retry
		// that ultimately succeeded.
		//
		// Real Job failures (backoff limit exhausted, pod
		// Failed) are still caught: the pod phase would be Failed
		// (not Succeeded) and the container's current state would
		// not be terminated{exitCode:0}.
		//
		// Charts that exhibit benign Job retries today:
		//   - airflow (run-airflow-migrations races postgres
		//     readiness when rendered as a regular resource via
		//     `migrateDatabaseJob.useHelmHooks: false`; first
		//     attempt fails with a transient connection error,
		//     restartPolicy:OnFailure retries, alembic completes,
		//     exit 0). See verity-org/verity#400.
		jobOwned := isOwnedByJob(p.Metadata.OwnerReferences)
		podSucceeded := p.Status.Phase == "Succeeded"

		// Main containers: any restart is a hard failure. The whole
		// point of the no-restart gate is to catch CrashLoopBackOff
		// or OOMKill on long-running workload containers during the
		// post-install settle window.
		for i := range p.Status.ContainerStatuses {
			cs := &p.Status.ContainerStatuses[i]
			if cs.RestartCount == 0 {
				continue
			}
			if jobOwned && podSucceeded && initContainerCompletedCleanly(cs) {
				continue
			}
			failures = append(failures, formatRestartFailure(p.Metadata.Namespace, p.Metadata.Name, cs, "container"))
		}
		// InitContainers: a benign init flake (e.g. waiting for
		// CRDs registered by a sibling pod, transient API server
		// unavailability during cluster warm-up) shows up as
		// restartCount>0 with the init STILL completing successfully
		// (state.terminated.exitCode == 0). The main container is
		// then running normally and the cluster is healthy — this
		// is NOT what the no-restart gate is meant to catch.
		//
		// Real CrashLoopBackOff in an initContainer still trips the
		// gate: it would have state.waiting.reason ==
		// "CrashLoopBackOff" (or a still-running / failed-terminated
		// state), so the exemption below intentionally does NOT
		// apply.
		//
		// Charts that exhibit benign init flakes today:
		//   - crossplane (init waits for CRDs that the chart's own
		//     CoreCRDs step installs; first attempt sometimes loses
		//     a race with kube-apiserver readiness and is restarted
		//     once by the kubelet)
		for i := range p.Status.InitContainerStatuses {
			cs := &p.Status.InitContainerStatuses[i]
			if cs.RestartCount == 0 {
				continue
			}
			if initContainerCompletedCleanly(cs) {
				continue
			}
			failures = append(failures, formatRestartFailure(p.Metadata.Namespace, p.Metadata.Name, cs, "initContainer"))
		}
	}
	return failures
}

// initContainerCompletedCleanly reports whether an initContainer is
// currently in the terminated{exitCode:0} state — i.e. the init
// succeeded, even if it took more than one attempt to get there.
// Used by collectRestartFailures to exempt benign init-phase flakes
// from the no-restart gate. A nil-terminated state (running /
// waiting / failed-terminated) returns false, preserving the gate
// for real crash loops.
//
// Also reused by collectRestartFailures for the Job-owned-pod
// main-container exemption: the function's contract — "terminated
// cleanly with exitCode 0" — applies equally to a main container
// in a Succeeded Job pod whose first attempt failed and second
// attempt succeeded.
func initContainerCompletedCleanly(cs *containerStatus) bool {
	t := cs.State.Terminated
	return t != nil && t.ExitCode == 0
}

// isOwnedByJob reports whether a pod has a controller owner-reference
// of kind "Job". Used by collectRestartFailures to admit
// main-container restartCount>0 on one-shot Job pods that ultimately
// succeeded. We intentionally do NOT match "CronJob" here — that
// would be the CronJob controller, NOT the per-execution Job; pods
// created by CronJob runs still have a direct `Kind: Job` owner.
func isOwnedByJob(refs []struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}) bool {
	for _, ref := range refs {
		if ref.Kind == "Job" {
			return true
		}
	}
	return false
}

func formatRestartFailure(namespace, podName string, cs *containerStatus, kind string) string {
	reason := "unknown"
	msg := ""
	if cs.LastTerminationState.Terminated != nil {
		reason = cs.LastTerminationState.Terminated.Reason
		msg = cs.LastTerminationState.Terminated.Message
	}
	return fmt.Sprintf(
		"pod=%s/%s %s=%s restarts=%d reason=%s msg=%q",
		namespace, podName, kind, cs.Name, cs.RestartCount, reason, truncate(msg, 200),
	)
}

// CollectNamespaceImages returns the deduplicated list of container
// images observed in the given namespace (main + init containers
// across all pods). It is exposed for the test driver's
// missing-allowlist branch in main_test.go: after AssertImageOrigin
// returns errAllowlistMissing, the driver re-collects images and
// asks classifyMissingAllowlist whether the absence is acceptable.
//
// The function does NOT consult any allowlist — it is a pure
// observation step. Policy lives in classifyMissingAllowlist.
func CollectNamespaceImages(ctx context.Context, h *Harness, namespace string) ([]string, error) {
	pods, err := getPods(ctx, h, namespace)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var out []string
	// Inner helper to avoid the per-pod slice allocation+copy that an
	// `append(append([]T{}, a...), b...)` would incur.
	add := func(statuses []containerStatus) {
		for _, cs := range statuses {
			if cs.Image == "" {
				continue
			}
			if _, ok := seen[cs.Image]; ok {
				continue
			}
			seen[cs.Image] = struct{}{}
			out = append(out, cs.Image)
		}
	}
	for _, p := range pods.Items {
		add(p.Status.ContainerStatuses)
		add(p.Status.InitContainerStatuses)
	}
	return out, nil
}

// classifyMissingAllowlist implements the SCR-001 missing-allowlist
// policy: when an allowlist file is absent, hard-fail iff any
// observed image is non-verity-prefixed. If every image is already
// verity-prefixed the chart genuinely doesn't need an allowlist and
// the absence is a clean pass.
//
// Returns the slice of offending (non-verity) images. An empty
// return slice means "missing-allowlist is acceptable for this
// namespace". A non-empty return slice means "the test driver must
// hard-fail and name allowlistPath as the file the chart needs".
//
// This helper is unit-tested without a cluster; the driver in
// main_test.go is responsible for fetching images via
// CollectNamespaceImages and turning the slice into a t.Errorf
// (we use Errorf, not Fatalf, so a single missing allowlist does
// not abort the rest of the chart's checks).
func classifyMissingAllowlist(images []string) []string {
	var offenders []string
	for _, img := range images {
		if strings.HasPrefix(img, verityRegistryPrefix) {
			continue
		}
		offenders = append(offenders, img)
	}
	return offenders
}

func AssertImageOrigin(ctx context.Context, h *Harness, namespace, allowlistPath string) error {
	pods, err := getPods(ctx, h, namespace)
	if err != nil {
		return err
	}
	allow, err := loadAllowlist(allowlistPath)
	if err != nil {
		return err
	}
	violations := collectImageViolations(pods, allow)
	if len(violations) > 0 {
		return fmt.Errorf("%w: count=%d namespace=%s allowlist=%s\n  %s",
			errImageNotAccepted, len(violations), namespace, allowlistPath, strings.Join(violations, "\n  "))
	}
	return nil
}

func collectImageViolations(pods *podList, allow []string) []string {
	var violations []string
	for _, p := range pods.Items {
		all := append(append([]containerStatus{}, p.Status.ContainerStatuses...), p.Status.InitContainerStatuses...)
		for _, cs := range all {
			if isAccepted(cs.Image, cs.ImageID, allow) {
				continue
			}
			// Diagnostic message uses Image (raw kubelet value) so
			// the bare-digest case is still visible in CI logs;
			// allowlist matching, however, has already consulted
			// ImageID as a fallback above.
			violations = append(violations, fmt.Sprintf(
				"pod=%s/%s container=%s image=%s",
				p.Metadata.Namespace, p.Metadata.Name, cs.Name, cs.Image,
			))
		}
	}
	return violations
}

// isAccepted returns true when the container is sourced from the
// verity registry OR matches an allowlist prefix.
//
// Acceptance order:
//
//  1. verityRegistryPrefix short-circuit on `image`
//  2. allowlist prefix match on `image`
//  3. bare-digest fallback: if `image` looks like "sha256:..." (no
//     registry path because kubelet/containerd normalised it), we
//     re-run the verity short-circuit and allowlist match against
//     `imageID`, which still carries the canonical registry-prefixed
//     digest reference.
//
// imageID may be empty when the test calls isAccepted with synthetic
// data; in that case the fallback degrades cleanly to the original
// image-only behaviour.
func isAccepted(image, imageID string, allow []string) bool {
	if image == "" {
		return false
	}
	if strings.HasPrefix(image, verityRegistryPrefix) {
		return true
	}
	for _, prefix := range allow {
		if strings.HasPrefix(image, prefix) {
			return true
		}
	}
	// Bare-digest fallback. kubelet sometimes records
	// status.containerStatuses[].image as just "sha256:<hex>" when
	// the pod spec pinned the image by tag+digest. In that case the
	// allowlist prefixes (which are repository paths) cannot match,
	// but imageID still carries "<repo>@sha256:<hex>" so the same
	// prefix logic works against it. See containerStatus.Image
	// docstring for the upstream cause.
	if strings.HasPrefix(image, "sha256:") && imageID != "" {
		if strings.HasPrefix(imageID, verityRegistryPrefix) {
			return true
		}
		for _, prefix := range allow {
			if strings.HasPrefix(imageID, prefix) {
				return true
			}
		}
	}
	return false
}

func loadAllowlist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Wrap errAllowlistMissing so callers can branch via
			// errors.Is while still seeing the requested path in
			// the error message for diagnostics.
			return nil, fmt.Errorf("%w: %s", errAllowlistMissing, path)
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, s.Err()
}

func getPods(ctx context.Context, h *Harness, namespace string) (*podList, error) {
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", h.KubeconfigPath,
		"get", "pods",
		"--namespace", namespace,
		"--output=json",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		stderr := strings.TrimSpace(string(out))
		if stderr != "" {
			return nil, fmt.Errorf("kubectl get pods -n %s: %s: %w", namespace, stderr, err)
		}
		return nil, fmt.Errorf("kubectl get pods -n %s: %w", namespace, err)
	}
	var pods podList
	if err := json.Unmarshal(out, &pods); err != nil {
		return nil, fmt.Errorf("decode pod list: %w", err)
	}
	return &pods, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
