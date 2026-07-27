package repositoryops

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	ErrInvalidWorkflowOutput = errors.New("invalid GitHub workflow output")
	workflowKeyPattern       = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,127}$`)
)

type WorkflowValue struct {
	Name  string
	Value string
}

func AppendWorkflowValues(path string, values []WorkflowValue) (err error) {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: output path is required", ErrInvalidWorkflowOutput)
	}
	for _, value := range values {
		if !workflowKeyPattern.MatchString(value.Name) || strings.ContainsAny(value.Value, "\r\n\x00") {
			return fmt.Errorf("%w: %q", ErrInvalidWorkflowOutput, value.Name)
		}
	}
	if info, statErr := os.Lstat(path); statErr == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %q is not a regular file", ErrInvalidWorkflowOutput, path)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect workflow output %q: %w", path, statErr)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open workflow output %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close workflow output %q: %w", path, closeErr)
		}
	}()
	for _, value := range values {
		if _, err := fmt.Fprintf(file, "%s=%s\n", value.Name, value.Value); err != nil {
			return fmt.Errorf("write workflow output %q: %w", path, err)
		}
	}
	return nil
}
