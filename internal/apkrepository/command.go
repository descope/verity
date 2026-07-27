package apkrepository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type command struct {
	name      string
	args      []string
	dir       string
	stdout    io.Writer
	sensitive bool
}

type commandResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type commandRunner interface {
	Run(context.Context, *command) (commandResult, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, request *command) (commandResult, error) {
	cmd := exec.CommandContext(ctx, trustedCommandPath(request.name), request.args...)
	cmd.Dir = request.dir
	cmd.Env = commandEnvironment(request.name, os.Environ())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if request.stdout == nil {
		cmd.Stdout = &stdout
	} else {
		cmd.Stdout = request.stdout
	}
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := commandResult{stdout: stdout.Bytes(), stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result, nil
	}
	return commandResult{}, fmt.Errorf("start %s: %w", request.name, err)
}

func runRequired(ctx context.Context, runner commandRunner, request *command) (commandResult, error) {
	result, err := runner.Run(ctx, request)
	if err != nil {
		return commandResult{}, err
	}
	if result.exitCode != 0 {
		if request.sensitive {
			return commandResult{}, fmt.Errorf("%w: %s exited %d", errCommandFailed, request.name, result.exitCode)
		}
		details := strings.TrimSpace(string(append(result.stdout, result.stderr...)))
		return commandResult{}, fmt.Errorf("%w: %s exited %d: %s", errCommandFailed, request.name, result.exitCode, details)
	}
	return result, nil
}

func trustedCommandPath(name string) string {
	switch name {
	case "apk":
		return "/sbin/apk"
	case "gh":
		return "/usr/bin/gh"
	case "melange":
		return "/usr/bin/melange"
	default:
		return name
	}
}

func commandEnvironment(name string, environment []string) []string {
	allowed := map[string]string{}
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		switch key {
		case "SSL_CERT_DIR", "SSL_CERT_FILE":
			allowed[key] = entry
		case "GH_TOKEN", "GITHUB_TOKEN":
			if name == "gh" {
				allowed[key] = entry
			}
		}
	}
	result := []string{"PATH=/usr/bin:/bin:/sbin"}
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if entry, ok := allowed[key]; ok {
			result = append(result, entry)
		}
	}
	return result
}
