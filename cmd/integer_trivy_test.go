package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type intTrivyResult struct {
	output   string
	exitCode int
}

type intTrivyFake struct {
	attempts func() int
	args     func() []string
}

func intFakeTrivySequence(t *testing.T, results []intTrivyResult) intTrivyFake {
	t.Helper()
	require.NotEmpty(t, results)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "trivy")
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	script.WriteString("count_file=\"$0.count\"\n")
	script.WriteString("attempt=0\n")
	script.WriteString("if [ -f \"$count_file\" ]; then attempt=$(cat \"$count_file\"); fi\n")
	script.WriteString("attempt=$((attempt + 1))\n")
	script.WriteString("printf '%s\\n' \"$attempt\" > \"$count_file\"\n")
	script.WriteString("printf '%s\\n' \"$@\" > \"$0.args\"\n")
	script.WriteString("case \"$attempt\" in\n")
	for index, result := range results {
		fmt.Fprintf(&script, "%d) printf '%%s\\n' %q >&2; exit %d ;;\n", index+1, result.output, result.exitCode)
	}
	last := results[len(results)-1]
	fmt.Fprintf(&script, "*) printf '%%s\\n' %q >&2; exit %d ;;\n", last.output, last.exitCode)
	script.WriteString("esac\n")

	require.NoError(t, os.WriteFile(scriptPath, []byte(script.String()), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	return intTrivyFake{
		attempts: func() int {
			t.Helper()
			data, err := os.ReadFile(scriptPath + ".count")
			require.NoError(t, err)
			attempts, err := strconv.Atoi(strings.TrimSpace(string(data)))
			require.NoError(t, err)
			return attempts
		},
		args: func() []string {
			t.Helper()
			data, err := os.ReadFile(scriptPath + ".args")
			require.NoError(t, err)
			return strings.Fields(string(data))
		},
	}
}

func TestIntegerTrivyGate_Retries_WhenVulnerabilityDBDownloadFails(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "404", output: "failed to download vulnerability DB: unexpected status code 404"},
		{name: "download", output: "failed to download vulnerability DB: download error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := intFakeTrivySequence(t, []intTrivyResult{
				{output: tt.output, exitCode: 1},
				{exitCode: 0},
			})
			var sleeps int

			err := integerTrivyGateWithSleeper(context.Background(), "image.tar", "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL", func(context.Context, time.Duration) error {
				sleeps++
				return nil
			})

			require.NoError(t, err)
			assert.Equal(t, 2, fake.attempts())
			assert.Equal(t, 1, sleeps)
			assert.Equal(t, []string{"image", "--input", "image.tar", "--exit-code", "1", "--severity", "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL", "--vuln-type", "os,library", "--format", "table"}, fake.args())
		})
	}
}

func TestIntegerTrivyGate_FailsWithoutRetry_WhenVulnerabilitiesFound(t *testing.T) {
	fake := intFakeTrivySequence(t, []intTrivyResult{{
		output:   "Total: 1 (UNKNOWN: 0, LOW: 0, MEDIUM: 0, HIGH: 1, CRITICAL: 0)",
		exitCode: 1,
	}})

	err := integerTrivyGateWithSleeper(context.Background(), "image.tar", "HIGH,CRITICAL", func(context.Context, time.Duration) error {
		t.Fatal("sleeper called for a vulnerability finding")
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to publish")
	assert.Equal(t, 1, fake.attempts())
}

func TestIntegerTrivyGate_FailsAfterBoundedRetries_WhenVulnerabilityDBRemainsUnavailable(t *testing.T) {
	fake := intFakeTrivySequence(t, []intTrivyResult{{
		output:   "failed to download vulnerability DB: unexpected status code 404",
		exitCode: 1,
	}})
	var sleeps int

	err := integerTrivyGateWithSleeper(context.Background(), "image.tar", "HIGH,CRITICAL", func(context.Context, time.Duration) error {
		sleeps++
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "vulnerability database unavailable after 3 attempts")
	assert.Equal(t, integerTrivyMaxAttempts, fake.attempts())
	assert.Equal(t, integerTrivyMaxAttempts-1, sleeps)
}

func TestIntegerTrivyGate_StopsRetrying_WhenContextIsCanceled(t *testing.T) {
	fake := intFakeTrivySequence(t, []intTrivyResult{{
		output:   "failed to download vulnerability DB: unexpected status code 404",
		exitCode: 1,
	}})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	err := integerTrivyGateWithSleeper(ctx, "image.tar", "HIGH,CRITICAL", func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, fake.attempts())
}
