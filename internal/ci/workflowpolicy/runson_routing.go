package workflowpolicy

import "strings"

const buildVerityTrustedRunnerRoute = "${{ (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) && format('runs-on={0}-{1}-{2}/runner=ci-large-x64', github.run_id, github.run_attempt, github.job) || 'ubuntu-24.04' }}"

type runsOnRoute struct {
	workflow string
	job      string
}

type runsOnRouteExpectation struct {
	label        string
	actionIf     string
	missingError string
}

var approvedRunsOnRoutes = map[runsOnRoute]string{
	{workflow: "build-verity-protected.yaml", job: "build"}:               "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=ci-large-x64",
	{workflow: "ci.yaml", job: "test"}:                                    "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=ci-large-x64",
	{workflow: "chart-integration.yaml", job: "chart-test"}:               "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=chart-x64",
	{workflow: "pr-test.yaml", job: "integer-smoke-test"}:                 "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=integer-${{ matrix.arch }}",
	{workflow: "pr-test.yaml", job: "integer-build-changed"}:              "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=integer-${{ matrix.arch }}",
	{workflow: "pr-test.yaml", job: "copa-patching-changed"}:              "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=buildkit-x64",
	{workflow: "pr-test.yaml", job: "copa-patching-regression"}:           "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=buildkit-x64",
	{workflow: "integer-build-image-reusable.yaml", job: "melange-build"}: "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=${{ matrix.runner_profile }}",
	{workflow: "integer-build-image-reusable.yaml", job: "build"}:         "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=integer-amd64",
	{workflow: "patch-image.yaml", job: "scan"}:                           "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=buildkit-x64",
	{workflow: "patch-image.yaml", job: "patch"}:                          "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=${{ matrix.runner_profile }}",
	{workflow: "orchestrator.yaml", job: "prepare"}:                       "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/runner=${{ matrix.runner_profile }}",
}

var fallbackRunsOnRoutes = map[runsOnRoute]string{
	{workflow: "build-verity.yaml", job: "build"}: buildVerityTrustedRunnerRoute,
}

func validateRunsOnRouting(workflows []workflowFile) []Violation {
	violations := make([]Violation, 0)
	seen := make(map[runsOnRoute]bool, len(approvedRunsOnRoutes)+len(fallbackRunsOnRoutes))
	for fileIndex := range workflows {
		file := &workflows[fileIndex]
		for jobName := range file.Workflow.Jobs {
			job := file.Workflow.Jobs[jobName]
			if file.Name == runsOnSmokeWorkflowName && jobName == runsOnCanaryJobName {
				continue
			}
			route := runsOnRoute{workflow: file.Name, job: jobName}
			expectation, approved := expectedRunsOnRoute(route)
			if !approved {
				if len(job.RunsOn) == 1 && strings.Contains(job.RunsOn[0], "runs-on=") {
					violations = append(violations, runsOnRouteViolation(file.Name, jobName, "job is not approved to use RunsOn"))
				}
				continue
			}
			seen[route] = true
			violations = append(violations, validateExpectedRunsOnRoute(file.Name, jobName, &job, route, expectation)...)
		}
	}
	violations = append(violations, missingRunsOnRoutes(seen)...)
	return violations
}

func expectedRunsOnRoute(route runsOnRoute) (runsOnRouteExpectation, bool) {
	if label, exists := approvedRunsOnRoutes[route]; exists {
		return runsOnRouteExpectation{label: label, missingError: "required RunsOn capacity route is missing"}, true
	}
	label, exists := fallbackRunsOnRoutes[route]
	return runsOnRouteExpectation{
		label:        label,
		actionIf:     "${{ " + runsOnTrustedExecution + " }}",
		missingError: "required trust-aware RunsOn capacity route is missing",
	}, exists
}

func validateExpectedRunsOnRoute(
	workflowName string,
	jobName string,
	job *workflowJob,
	route runsOnRoute,
	expectation runsOnRouteExpectation,
) []Violation {
	if len(job.RunsOn) != 1 || job.RunsOn[0] != expectation.label {
		return []Violation{runsOnRouteViolation(workflowName, jobName, "job must use its exact reviewed RunsOn capacity route")}
	}
	violations := validateRunsOnRouteTrust(workflowName, jobName, job, route, expectation.actionIf != "")
	return append(violations, validateRunsOnProductionSteps(workflowName, jobName, job.Steps, expectation.actionIf)...)
}

func validateRunsOnRouteTrust(
	workflowName string,
	jobName string,
	job *workflowJob,
	route runsOnRoute,
	trustIsInRunnerExpression bool,
) []Violation {
	if trustIsInRunnerExpression {
		return nil
	}
	if route.workflow == protectedBuildVerityWorkflowName && route.job == "build" {
		if normalizeExpression(job.If) != protectedBuildGate {
			return []Violation{runsOnRouteViolation(workflowName, jobName, "protected RunsOn build must retain its exact protected-source gate")}
		}
		return nil
	}
	if !strings.Contains(normalizeExpression(job.If), normalizeExpression(runsOnTrustedExecution)) {
		return []Violation{runsOnRouteViolation(workflowName, jobName, "job must reject untrusted pull-request code")}
	}
	return nil
}

func missingRunsOnRoutes(seen map[runsOnRoute]bool) []Violation {
	violations := make([]Violation, 0)
	for route := range approvedRunsOnRoutes {
		if !seen[route] {
			expectation, _ := expectedRunsOnRoute(route)
			violations = append(violations, runsOnRouteViolation(route.workflow, route.job, expectation.missingError))
		}
	}
	for route := range fallbackRunsOnRoutes {
		if !seen[route] {
			expectation, _ := expectedRunsOnRoute(route)
			violations = append(violations, runsOnRouteViolation(route.workflow, route.job, expectation.missingError))
		}
	}
	return violations
}

func validateRunsOnProductionSteps(workflowName, jobName string, steps []workflowStep, actionIf string) []Violation {
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
			if !exactRunsOnActionWithIf(step, actionIf) {
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
