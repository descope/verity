package workflowpolicy

import "strings"

type runsOnRoute struct {
	workflow string
	job      string
}

var approvedRunsOnRoutes = map[runsOnRoute]string{
	{workflow: "ci.yaml", job: "test"}:                                    "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=ci-large-x64",
	{workflow: "chart-integration.yaml", job: "chart-test"}:               "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=chart-x64",
	{workflow: "pr-test.yaml", job: "integer-smoke-test"}:                 "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=integer-${{ matrix.arch }}",
	{workflow: "pr-test.yaml", job: "integer-build-changed"}:              "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=integer-${{ matrix.arch }}",
	{workflow: "pr-test.yaml", job: "copa-patching-changed"}:              "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=buildkit-x64",
	{workflow: "pr-test.yaml", job: "copa-patching-regression"}:           "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=buildkit-x64",
	{workflow: "integer-build-image-reusable.yaml", job: "melange-build"}: "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=${{ matrix.runner_profile }}",
	{workflow: "integer-build-image-reusable.yaml", job: "build"}:         "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=integer-x64",
	{workflow: "patch-image.yaml", job: "scan"}:                           "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=buildkit-x64",
	{workflow: "patch-image.yaml", job: "patch"}:                          "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=${{ matrix.runner_profile }}",
	{workflow: "orchestrator.yaml", job: "prepare"}:                       "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=${{ matrix.runner_profile }}",
}

func validateRunsOnRouting(workflows []workflowFile) []Violation {
	violations := make([]Violation, 0)
	for fileIndex := range workflows {
		file := &workflows[fileIndex]
		for jobName := range file.Workflow.Jobs {
			job := file.Workflow.Jobs[jobName]
			if len(job.RunsOn) != 1 || !strings.HasPrefix(job.RunsOn[0], "runs-on=") {
				continue
			}
			if file.Name == runsOnSmokeWorkflowName && jobName == runsOnCanaryJobName {
				continue
			}
			route := runsOnRoute{workflow: file.Name, job: jobName}
			expected, approved := approvedRunsOnRoutes[route]
			if !approved || job.RunsOn[0] != expected {
				violations = append(violations, runsOnRouteViolation(file.Name, jobName, "job must use its exact reviewed RunsOn capacity route"))
				continue
			}
			if !strings.Contains(normalizeExpression(job.If), normalizeExpression(runsOnTrustedExecution)) {
				violations = append(violations, runsOnRouteViolation(file.Name, jobName, "job must reject untrusted pull-request code"))
			}
			violations = append(violations, validateRunsOnProductionSteps(file.Name, jobName, job.Steps)...)
		}
	}
	return violations
}

func validateRunsOnProductionSteps(workflowName, jobName string, steps []workflowStep) []Violation {
	hardenIndex := -1
	actionIndex := -1
	actionCount := 0
	for index := range steps {
		step := &steps[index]
		switch actionName(step.Uses) {
		case "step-security/harden-runner":
			hardenIndex = index
		case runsOnActionName:
			actionIndex = index
			actionCount++
			if !exactRunsOnAction(step) {
				return []Violation{runsOnRouteViolation(workflowName, jobName, "RunsOn action must use the reviewed release and safe telemetry inputs")}
			}
		}
		if usesForbiddenRunsOnFeature(step) {
			return []Violation{runsOnRouteViolation(workflowName, jobName, "persistent or shared RunsOn features require a separate review")}
		}
	}
	if hardenIndex < 0 || actionCount != 1 || actionIndex <= hardenIndex {
		return []Violation{runsOnRouteViolation(workflowName, jobName, "runner hardening must precede exactly one RunsOn action")}
	}
	return nil
}

func runsOnRouteViolation(workflowName, jobName, detail string) Violation {
	return Violation{Rule: RuleRunsOnBoundary, Workflow: workflowName, Job: jobName, Detail: detail}
}
