package workflowpolicy

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var (
	pythonCommandPattern = regexp.MustCompile(`(^|[[:space:];|&])python(?:3(?:\.\d+)?)?(\s|$)`)
	controlFlowPattern   = regexp.MustCompile(`^(if|then|elif|else|fi|for|while|until|case|esac|select|function)\b`)
	assignmentPattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

func validateGoOwnedLogic(workflows []workflowFile) []Violation {
	var violations []Violation
	for index := range workflows {
		file := &workflows[index]
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				if reason := workflowStepLogicViolation(file.Name, jobName, step); reason != "" {
					violations = append(violations, Violation{
						Rule: RuleGoOwnedLogic, Workflow: file.Name, Job: jobName,
						Detail: fmt.Sprintf("step %q: %s", stepLabel(step), reason),
					})
				}
			}
		}
	}
	return violations
}

func workflowStepLogicViolation(workflowName, jobName string, step *workflowStep) string {
	if isBoundedSignerStdinBridge(workflowName, jobName, step) {
		return ""
	}
	return workflowLogicViolation(step.Run, step.Shell)
}

func workflowLogicViolation(run, shell string) string {
	trimmed := strings.TrimSpace(run)
	if trimmed == "" {
		return ""
	}
	if reason := workflowProgramReason(trimmed, shell); reason != "" {
		return reason
	}

	normalized := strings.ReplaceAll(trimmed, "\\\n", " ")
	for rawLine := range strings.SplitSeq(normalized, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if reason := shellLineReason(line); reason != "" {
			return reason
		}
	}
	return ""
}

func workflowProgramReason(run, shell string) string {
	lower := strings.ToLower(run)
	if pythonCommandPattern.MatchString(lower) || strings.HasPrefix(strings.ToLower(strings.TrimSpace(shell)), "python") {
		return "Python policy or orchestration must be implemented in Go"
	}
	if referencesWorkflowScript(lower) {
		return "repository-owned shell/Python scripts must be replaced by a typed Go command"
	}
	if strings.Contains(lower, "bash -c") || strings.Contains(lower, "bash -e") ||
		strings.Contains(lower, "sh -c") || strings.Contains(lower, "sh -e") {
		return "inline shell programs are not trivial process invocation"
	}
	return ""
}

func shellLineReason(line string) string {
	lower := strings.ToLower(line)
	if controlFlowPattern.MatchString(lower) || line == "{" || line == "}" {
		return "shell control flow must be implemented in Go"
	}
	if strings.Contains(line, "$(") || strings.Contains(line, "`") || strings.Contains(line, "<<") {
		return "shell evaluation or heredocs must be implemented in Go"
	}
	if containsPipelineOrRedirect(line) {
		return "shell pipelines and redirections must be implemented in Go"
	}
	fields := strings.Fields(line)
	if len(fields) == 1 && assignmentPattern.MatchString(fields[0]) {
		return "standalone shell state must be implemented in Go"
	}
	return ""
}

func referencesWorkflowScript(run string) bool {
	for _, command := range splitShellCommands(run) {
		invocation := parseShellInvocation(command)
		if invocation.executable >= 0 && path.Base(command[invocation.executable]) == "shellcheck" {
			continue
		}
		for _, token := range command {
			cleaned := strings.Trim(token, "'\"\\")
			if !strings.Contains(cleaned, ".github/scripts/") && !strings.HasPrefix(cleaned, "scripts/") {
				continue
			}
			if strings.HasSuffix(cleaned, ".sh") || strings.HasSuffix(cleaned, ".py") {
				return true
			}
		}
	}
	return false
}

func containsPipelineOrRedirect(line string) bool {
	withoutExpressions := line
	for {
		start := strings.Index(withoutExpressions, "${{")
		if start < 0 {
			break
		}
		end := strings.Index(withoutExpressions[start+3:], "}}")
		if end < 0 {
			break
		}
		withoutExpressions = withoutExpressions[:start] + withoutExpressions[start+3+end+2:]
	}
	for index := 0; index < len(withoutExpressions); index++ {
		character := rune(withoutExpressions[index])
		switch character {
		case '|':
			if index+1 < len(withoutExpressions) && withoutExpressions[index+1] == '|' {
				index++
				continue
			}
			return true
		case '>', '<', ';':
			return true
		}
	}
	return false
}

func stepLabel(step *workflowStep) string {
	if step.Name != "" {
		return step.Name
	}
	if step.ID != "" {
		return step.ID
	}
	return "unnamed"
}
