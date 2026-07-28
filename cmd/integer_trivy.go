package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	if err := integerDownloadTrivyDatabase(ctx, trivyPath, sleep); err != nil {
		return err
	}

	command := exec.CommandContext(
		ctx, trivyPath, "image",
		"--skip-db-update",
		"--input", tarPath,
		"--exit-code", "1",
		"--severity", severity,
		"--vuln-type", "os,library",
		"--format", "table",
	)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("trivy gate: image has %s vulnerabilities — refusing to publish: %w", severity, err)
	}
	return nil
}

func integerDownloadTrivyDatabase(ctx context.Context, trivyPath string, sleep integerTrivySleeper) error {
	for attempt := 1; attempt <= integerTrivyMaxAttempts; attempt++ {
		command := exec.CommandContext(ctx, trivyPath, "image", "--download-db-only")
		command.Stdout = os.Stderr
		command.Stderr = os.Stderr
		if err := command.Run(); err == nil {
			return nil
		} else if attempt == integerTrivyMaxAttempts {
			return fmt.Errorf("trivy gate: vulnerability database unavailable after %d attempts: %w", attempt, err)
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("trivy gate: retrying vulnerability database download: %w", err)
		}
		if err := sleep(ctx, integerTrivyRetryDelay); err != nil {
			return fmt.Errorf("trivy gate: retrying vulnerability database download: %w", err)
		}
	}
	return nil
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
