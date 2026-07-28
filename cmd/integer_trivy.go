package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	integerTrivyMaxAttempts = 3
	integerTrivyRetryDelay  = time.Second
)

type integerTrivySleeper func(context.Context, time.Duration) error

// integerTrivyGate scans a locally-built image tarball and returns a non-nil
// error if Trivy finds vulnerabilities at any of the comma-separated
// severities. This is the publish gate: the caller MUST stop before pushing
// the image when this returns an error ("not clean, no go").
func integerTrivyGate(ctx context.Context, tarPath, severity string) error {
	return integerTrivyGateWithSleeper(ctx, tarPath, severity, integerTrivySleep)
}

func integerTrivyGateWithSleeper(ctx context.Context, tarPath, severity string, sleep integerTrivySleeper) error {
	trivyPath, err := exec.LookPath("trivy")
	if err != nil {
		return fmt.Errorf("trivy not found in PATH (install via mise): %w", err)
	}

	for attempt := 1; ; attempt++ {
		output, err := integerRunTrivy(ctx, trivyPath, tarPath, severity)
		if err == nil {
			return nil
		}
		databaseUnavailable := integerTrivyVulnerabilityDBDownloadFailure(output)
		if databaseUnavailable && attempt == integerTrivyMaxAttempts {
			return fmt.Errorf("trivy gate: vulnerability database unavailable after %d attempts: %w", attempt, err)
		}
		if !databaseUnavailable {
			return fmt.Errorf("trivy gate: image has %s vulnerabilities — refusing to publish: %w", severity, err)
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("trivy gate: retrying vulnerability database download: %w", err)
		}
		if err := sleep(ctx, integerTrivyRetryDelay); err != nil {
			return fmt.Errorf("trivy gate: retrying vulnerability database download: %w", err)
		}
	}
}

func integerRunTrivy(ctx context.Context, trivyPath, tarPath, severity string) (string, error) {
	var captured bytes.Buffer
	output := io.MultiWriter(os.Stderr, &captured)
	command := exec.CommandContext(
		ctx, trivyPath, "image",
		"--input", tarPath,
		"--exit-code", "1",
		"--severity", severity,
		"--vuln-type", "os,library",
		"--format", "table",
	)
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	return captured.String(), err
}

func integerTrivyVulnerabilityDBDownloadFailure(output string) bool {
	return strings.Contains(strings.ToLower(output), "failed to download vulnerability db")
}

func integerTrivySleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
