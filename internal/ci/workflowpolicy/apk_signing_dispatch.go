package workflowpolicy

import (
	"fmt"
	"strings"
	"unicode"
)

func validateProtectedDispatchRefs(workflows []workflowFile) []Violation {
	var violations []Violation
	for fileIndex := range workflows {
		file := &workflows[fileIndex]
		if !file.Workflow.On.WorkflowDispatch {
			continue
		}
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				if actionName(step.Uses) != "actions/checkout" {
					continue
				}
				ref := strings.TrimSpace(step.With["ref"])
				if ref == "" || ref == "main" || ref == "refs/heads/main" || exactGitHubSHAExpression(ref) {
					continue
				}
				violations = append(violations, Violation{Rule: RuleProtectedDispatch, Workflow: file.Name, Job: jobName, Detail: fmt.Sprintf("workflow_dispatch checkout ref %q must be protected main or github.sha", ref)})
			}
		}
	}
	return violations
}

func exactGitHubSHAExpression(value string) bool {
	body, err := gateExpressionBody(value)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(value), "${{") {
		return false
	}
	for strings.HasPrefix(strings.TrimSpace(body), "(") {
		unwrapped, ok := unwrapExactExpressionParentheses(body)
		if !ok {
			return false
		}
		body = unwrapped
	}
	body, ok := compactExpressionWhitespace(body)
	if !ok {
		return false
	}
	return body == "github.sha" || body == "github['sha']" || body == `github["sha"]`
}

func compactExpressionWhitespace(value string) (string, bool) {
	var compact strings.Builder
	quote := rune(0)
	for _, character := range value {
		if quote != 0 {
			compact.WriteRune(character)
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			compact.WriteRune(character)
			continue
		}
		if !unicode.IsSpace(character) {
			compact.WriteRune(character)
		}
	}
	return compact.String(), quote == 0
}

func unwrapExactExpressionParentheses(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '(' {
		return value, false
	}
	depth := 0
	quote := byte(0)
	for index := range len(value) {
		character := value[index]
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && index != len(value)-1 {
				return value, false
			}
			if depth < 0 {
				return value, false
			}
		}
	}
	if depth != 0 || quote != 0 {
		return value, false
	}
	return strings.TrimSpace(value[1 : len(value)-1]), true
}
