package workflowpolicy

import "strings"

const (
	runsOnJobNamespace          = "runs-on=${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}"
	runsOnControlX64Route       = runsOnJobNamespace + "/family=c8i/cpu=4/ram=8/image=ubuntu24-full-x64/volume=40gb:gp3/extras=otel/spot=false"
	runsOnCILargeX64Route       = runsOnJobNamespace + "/family=c8i+m8i/cpu=16/ram=32/image=ubuntu24-full-x64/volume=100gb:gp3/extras=otel/spot=false"
	runsOnBuildKitX64Route      = runsOnJobNamespace + "/family=c8i+m8i/cpu=16/ram=32/image=ubuntu24-full-x64/volume=150gb:gp3/extras=otel/spot=false"
	runsOnChartX64Route         = runsOnBuildKitX64Route
	runsOnIntegerAMD64Route     = runsOnJobNamespace + "/family=c8i+m8i/cpu=32/ram=64/image=ubuntu24-full-x64/volume=200gb:gp3/extras=otel/spot=false"
	runsOnPRIntegerRoute        = runsOnJobNamespace + "/family=${{ matrix.arch == 'amd64' && 'c8i+m8i' || 'c8g+m8g' }}/cpu=32/ram=64/image=ubuntu24-full-${{ matrix.arch == 'amd64' && 'x64' || 'arm64' }}/volume=200gb:gp3/extras=otel/spot=false"
	runsOnMelangeRoute          = runsOnJobNamespace + "/family=${{ matrix.arch == 'x86_64' && 'c8i+m8i' || 'c8g+m8g' }}/cpu=32/ram=64/image=ubuntu24-full-${{ matrix.arch == 'x86_64' && 'x64' || 'arm64' }}/volume=200gb:gp3/extras=otel/spot=false"
	runsOnBuildKitPlatformRoute = runsOnJobNamespace + "/family=${{ matrix.platform == 'linux/amd64' && 'c8i+m8i' || 'c8g+m8g' }}/cpu=16/ram=32/image=ubuntu24-full-${{ matrix.platform == 'linux/amd64' && 'x64' || 'arm64' }}/volume=150gb:gp3/extras=otel/spot=false"
	runsOnBuildKitProfileRoute  = runsOnJobNamespace + "/family=${{ matrix.runner_profile == 'buildkit-x64' && 'c8i+m8i' || 'c8g+m8g' }}/cpu=16/ram=32/image=ubuntu24-full-${{ matrix.runner_profile == 'buildkit-x64' && 'x64' || 'arm64' }}/volume=150gb:gp3/extras=otel/spot=false"
)

type runsOnRoute struct {
	workflow string
	job      string
}

type runsOnRouteExpectation struct {
	label         string
	missingError  string
	validateSteps bool
	requiresTrust bool
}

var approvedRunsOnRoutes = map[runsOnRoute]string{
	{workflow: "build-verity.yaml", job: "build"}:                         runsOnCILargeX64Route,
	{workflow: "build-verity-protected.yaml", job: "build"}:               runsOnCILargeX64Route,
	{workflow: "ci.yaml", job: "test"}:                                    runsOnCILargeX64Route,
	{workflow: "chart-integration.yaml", job: "chart-test"}:               runsOnChartX64Route,
	{workflow: "codeql.yaml", job: "analyze"}:                             runsOnCILargeX64Route,
	{workflow: "pr-test.yaml", job: "integer-smoke-test"}:                 runsOnPRIntegerRoute,
	{workflow: "pr-test.yaml", job: "integer-build-changed"}:              runsOnPRIntegerRoute,
	{workflow: "pr-test.yaml", job: "copa-patching-changed"}:              runsOnBuildKitX64Route,
	{workflow: "pr-test.yaml", job: "copa-patching-regression"}:           runsOnBuildKitX64Route,
	{workflow: "integer-build-image-reusable.yaml", job: "melange-build"}: runsOnMelangeRoute,
	{workflow: "integer-build-image-reusable.yaml", job: "build"}:         runsOnIntegerAMD64Route,
	{workflow: "patch-image.yaml", job: "scan"}:                           runsOnBuildKitX64Route,
	{workflow: "patch-image.yaml", job: "patch"}:                          runsOnBuildKitPlatformRoute,
	{workflow: "orchestrator.yaml", job: "prepare"}:                       runsOnBuildKitProfileRoute,
}

func validateRunsOnRouting(workflows []workflowFile) []Violation {
	violations := make([]Violation, 0)
	seen := make(map[runsOnRoute]bool, len(approvedRunsOnRoutes))
	for fileIndex := range workflows {
		file := &workflows[fileIndex]
		for jobName := range file.Workflow.Jobs {
			job := file.Workflow.Jobs[jobName]
			if file.Name == runsOnSmokeWorkflowName && jobName == runsOnCanaryJobName {
				continue
			}
			route := runsOnRoute{workflow: file.Name, job: jobName}
			expectation, approved := expectedRunsOnRoute(route, &job)
			if !approved {
				if len(job.RunsOn) == 1 && isGitHubHostedRunner(job.RunsOn[0]) {
					violations = append(violations, runsOnRouteViolation(file.Name, jobName, "GitHub-hosted runners are forbidden"))
					continue
				}
				if len(job.RunsOn) == 1 && strings.Contains(job.RunsOn[0], "runs-on=") {
					violations = append(violations, runsOnRouteViolation(file.Name, jobName, "job is not approved to use RunsOn"))
				}
				continue
			}
			if expectation.label == runsOnControlX64Route && controlRouteRequiresTrust(file, jobName) {
				expectation.requiresTrust = true
			}
			seen[route] = true
			violations = append(violations, validateExpectedRunsOnRoute(file.Name, jobName, &job, route, expectation)...)
		}
	}
	violations = append(violations, missingRunsOnRoutes(seen)...)
	return violations
}

func controlRouteRequiresTrust(file *workflowFile, jobName string) bool {
	if file.Name == "chart-integration.yaml" && jobName == "chart-integration-result" {
		return false
	}
	return file.Workflow.On.PullRequest || file.Name == buildVerityWorkflowName
}

func expectedRunsOnRoute(route runsOnRoute, job *workflowJob) (runsOnRouteExpectation, bool) {
	if label, exists := approvedRunsOnRoutes[route]; exists {
		return runsOnRouteExpectation{
			label: label, missingError: "required RunsOn capacity route is missing",
			validateSteps: true, requiresTrust: true,
		}, true
	}
	if len(job.RunsOn) == 1 && job.RunsOn[0] == runsOnControlX64Route {
		return runsOnRouteExpectation{label: runsOnControlX64Route}, true
	}
	return runsOnRouteExpectation{}, false
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
	violations := validateRunsOnRouteTrust(workflowName, jobName, job, route, expectation.requiresTrust)
	if expectation.validateSteps {
		violations = append(violations, validateRunsOnProductionSteps(workflowName, jobName, job.Steps)...)
	}
	return violations
}

func validateRunsOnRouteTrust(
	workflowName string,
	jobName string,
	job *workflowJob,
	route runsOnRoute,
	requiresTrust bool,
) []Violation {
	if !requiresTrust {
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
			expectation, _ := expectedRunsOnRoute(route, &workflowJob{})
			violations = append(violations, runsOnRouteViolation(route.workflow, route.job, expectation.missingError))
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

func isGitHubHostedRunner(label string) bool {
	return strings.HasPrefix(label, "ubuntu-") || strings.HasPrefix(label, "windows-") ||
		strings.HasPrefix(label, "macos-")
}

func runsOnRouteViolation(workflowName, jobName, detail string) Violation {
	return Violation{Rule: RuleRunsOnBoundary, Workflow: workflowName, Job: jobName, Detail: detail}
}
