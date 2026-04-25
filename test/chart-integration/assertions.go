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
)

type podList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Status struct {
			ContainerStatuses     []containerStatus `json:"containerStatuses"`
			InitContainerStatuses []containerStatus `json:"initContainerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

type containerStatus struct {
	Name                 string `json:"name"`
	Image                string `json:"image"`
	RestartCount         int    `json:"restartCount"`
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
		all := append(append([]containerStatus{}, p.Status.ContainerStatuses...), p.Status.InitContainerStatuses...)
		for _, cs := range all {
			if cs.RestartCount == 0 {
				continue
			}
			reason := "unknown"
			msg := ""
			if cs.LastTerminationState.Terminated != nil {
				reason = cs.LastTerminationState.Terminated.Reason
				msg = cs.LastTerminationState.Terminated.Message
			}
			failures = append(failures, fmt.Sprintf(
				"pod=%s/%s container=%s restarts=%d reason=%s msg=%q",
				p.Metadata.Namespace, p.Metadata.Name, cs.Name, cs.RestartCount, reason, truncate(msg, 200),
			))
		}
	}
	return failures
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
			if isAccepted(cs.Image, allow) {
				continue
			}
			violations = append(violations, fmt.Sprintf(
				"pod=%s/%s container=%s image=%s",
				p.Metadata.Namespace, p.Metadata.Name, cs.Name, cs.Image,
			))
		}
	}
	return violations
}

func isAccepted(image string, allow []string) bool {
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
	return false
}

func loadAllowlist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
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
	out, err := cmd.Output()
	if err != nil {
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
