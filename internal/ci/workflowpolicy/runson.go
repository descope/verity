package workflowpolicy

import (
	"path/filepath"
	"slices"
	"strings"
)

const (
	runsOnSmokeWorkflowName = "runs-on-smoke.yaml"
	runsOnCanaryJobName     = "canary"
	runsOnProfileName       = "canary-x64"
	runsOnActionName        = "runs-on/action"
	runsOnActionReference   = "runs-on/action@4e5f72399b6b17f2e79c511c1b38a315a64d22dc"
	runsOnCanaryLabel       = runsOnJobNamespace + "/family=c8i+m8i/cpu=4/ram=8/image=ubuntu24-full-x64/volume=40gb:gp3/extras=otel/spot=false"
	runsOnTrustedIf         = "github.repository == 'verity-org/verity' && github.ref == 'refs/heads/main'"
	runsOnTrustedExecution  = "github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository"
	hardenRunnerReference   = "step-security/harden-runner@bf7454d06d71f1098171f2acdf0cd4708d7b5920"
	checkoutReference       = "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0"
)

func validateRunsOnBoundary(directory string, workflows []workflowFile) []Violation {
	repositoryRoot, repositoryLayout := runsOnRepositoryRoot(directory)
	if !repositoryLayout {
		return nil
	}
	return validateRunsOnRepository(repositoryRoot, workflows)
}

func runsOnRepositoryRoot(directory string) (string, bool) {
	cleaned := filepath.Clean(directory)
	githubDirectory := filepath.Dir(cleaned)
	if filepath.Base(cleaned) != "workflows" || filepath.Base(githubDirectory) != ".github" {
		return "", false
	}
	return filepath.Dir(githubDirectory), true
}

func validateRunsOnRepository(repositoryRoot string, workflows []workflowFile) []Violation {
	violations := validateRunsOnCatalog(repositoryRoot)
	violations = append(violations, validateRunsOnRouting(workflows)...)
	canary, exists := findWorkflow(workflows, runsOnSmokeWorkflowName)
	if !exists {
		return append(violations, runsOnViolation("", "required capability canary is missing"))
	}
	violations = append(violations, validateRunsOnWorkflow(&canary)...)
	return violations
}

func validateRunsOnWorkflow(file *workflowFile) []Violation {
	violations := make([]Violation, 0, 8)
	workflow := &file.Workflow
	if workflow.Name != "RunsOn Capability Smoke" || !exactRunsOnTrigger(workflow.On) {
		violations = append(violations, runsOnViolation("", "canary must be manual-only with its canonical name"))
	}
	if !buildOnceExactPermissions(workflow.Permissions, map[permissionScope]permissionLevel{}) {
		violations = append(violations, runsOnViolation("", "workflow permissions must be empty"))
	}
	if len(workflow.Env) != 0 {
		violations = append(violations, runsOnViolation("", "workflow environment must be empty"))
	}
	if len(workflow.Jobs) != 2 {
		violations = append(violations, runsOnViolation("", "canary workflow must contain only build-verity and canary jobs"))
	}
	buildJob := workflow.Jobs["build-verity"]
	canaryJob := workflow.Jobs[runsOnCanaryJobName]
	violations = append(violations, validateRunsOnBuildJob(&buildJob)...)
	violations = append(violations, validateRunsOnCanaryJob(&canaryJob)...)
	return violations
}

func exactRunsOnTrigger(trigger triggers) bool {
	return trigger.WorkflowDispatch && exactRunsOnDispatchInputs(trigger.DispatchInputs) &&
		!trigger.Push.Present && !trigger.PullRequest &&
		!trigger.PullRequestTarget && !trigger.Schedule && !trigger.WorkflowCall &&
		!trigger.WorkflowRun && !trigger.OtherEvent
}

func exactRunsOnDispatchInputs(inputs map[string]workflowDispatchInput) bool {
	if len(inputs) != 2 {
		return false
	}
	for _, name := range []string{"expected_aws_account", "expected_aws_region"} {
		input, exists := inputs[name]
		if !exists || !input.Required || input.Type != "string" || input.Default.set {
			return false
		}
	}
	return true
}

func validateRunsOnBuildJob(job *workflowJob) []Violation {
	expectedPermissions := map[permissionScope]permissionLevel{contentsScope: permissionRead}
	if job.Uses != "./.github/workflows/build-verity.yaml" ||
		!buildOnceExactPermissions(job.Permissions, expectedPermissions) ||
		len(job.With) != 1 || normalizeExpression(job.With["source_sha"]) != "${{github.sha}}" ||
		job.Secrets.set || len(job.Env) != 0 || len(job.Steps) != 0 || len(job.RunsOn) != 0 ||
		job.Environment.Kind != 0 || job.Strategy.Present ||
		normalizeExpression(job.If) != normalizeExpression(runsOnTrustedIf) || job.ContinueOnError.set {
		return []Violation{runsOnViolation("build-verity", "producer must remain the exact unprivileged current-run Verity build")}
	}
	return nil
}

func validateRunsOnCanaryJob(job *workflowJob) []Violation {
	violations := make([]Violation, 0, 6)
	expectedPermissions := map[permissionScope]permissionLevel{
		actionsScope:  permissionRead,
		contentsScope: permissionRead,
	}
	if len(job.RunsOn) != 1 || job.RunsOn[0] != runsOnCanaryLabel {
		violations = append(violations, runsOnViolation(runsOnCanaryJobName, "job must use the exact single-string canary-x64 label"))
	}
	if len(job.Needs) != 1 || job.Needs[0] != "build-verity" ||
		!buildOnceExactPermissions(job.Permissions, expectedPermissions) || job.Secrets.set ||
		len(job.Env) != 0 || job.Uses != "" || job.Container.Image != "" || len(job.Services) != 0 ||
		job.Environment.Kind != 0 || job.Strategy.Present ||
		normalizeExpression(job.If) != normalizeExpression(runsOnTrustedIf) || job.ContinueOnError.set {
		violations = append(violations, runsOnViolation(runsOnCanaryJobName, "job must remain secret-free and actions/contents read-only"))
	}
	violations = append(violations, validateRunsOnSteps(job.Steps)...)
	return violations
}

type runsOnStepIndexes struct {
	harden int
	action int
	verify int
	count  int
}

func validateRunsOnSteps(steps []workflowStep) []Violation {
	if len(steps) != 5 || !exactRunsOnHardenStep(&steps[0]) || !exactRunsOnCheckoutStep(&steps[2]) ||
		!exactRunsOnSetupStep(&steps[3]) || !exactRunsOnVerifyStep(&steps[4]) {
		return []Violation{runsOnViolation(runsOnCanaryJobName, "canary must contain only the five reviewed bootstrap steps")}
	}
	indexes, violations := inspectRunsOnSteps(steps)
	if len(violations) > 0 {
		return violations
	}
	if indexes.count != 1 || indexes.harden < 0 || indexes.action <= indexes.harden || indexes.verify <= indexes.action {
		return []Violation{runsOnViolation(runsOnCanaryJobName, "step order must be hardening, RunsOn activation, then typed host verification")}
	}
	if !exactRunsOnVerifyArguments(steps[indexes.verify].Run) {
		return []Violation{runsOnViolation(runsOnCanaryJobName, "typed host verification arguments must remain fail-closed")}
	}
	return nil
}

func exactRunsOnHardenStep(step *workflowStep) bool {
	return step.Uses == hardenRunnerReference && len(step.With) == 1 && step.With["egress-policy"] == "audit" &&
		len(step.Env) == 0 && !step.ContinueOnError.set && step.If == ""
}

func exactRunsOnCheckoutStep(step *workflowStep) bool {
	return step.Uses == checkoutReference && len(step.With) == 2 &&
		normalizeExpression(step.With["ref"]) == "${{github.sha}}" && step.With["persist-credentials"] == "false" &&
		len(step.Env) == 0 && !step.ContinueOnError.set && step.If == ""
}

func exactRunsOnSetupStep(step *workflowStep) bool {
	if step.Uses != "./.github/actions/setup-verity" || len(step.With) != 5 || len(step.Env) != 0 ||
		step.ContinueOnError.set || step.If != "" {
		return false
	}
	expected := map[string]string{
		"artifact-name":      "${{needs.build-verity.outputs.artifact-name}}",
		"artifact-digest":    "${{needs.build-verity.outputs.artifact-digest}}",
		"source-sha":         "${{needs.build-verity.outputs.source-sha}}",
		"build-key":          "${{needs.build-verity.outputs.build-key}}",
		"verify-attestation": "false",
	}
	for key, value := range expected {
		if normalizeExpression(step.With[key]) != value {
			return false
		}
	}
	return true
}

func exactRunsOnVerifyStep(step *workflowStep) bool {
	return step.Uses == "" && len(step.Env) == 2 &&
		normalizeExpression(step.Env["EXPECTED_AWS_ACCOUNT"]) == "${{inputs.expected_aws_account}}" &&
		normalizeExpression(step.Env["EXPECTED_AWS_REGION"]) == "${{inputs.expected_aws_region}}" &&
		!step.ContinueOnError.set && step.If == "" && step.Shell == "" && step.WorkingDirectory == "" &&
		exactRunsOnVerifyArguments(step.Run)
}

func inspectRunsOnSteps(steps []workflowStep) (runsOnStepIndexes, []Violation) {
	indexes := runsOnStepIndexes{harden: -1, action: -1, verify: -1}
	for index := range steps {
		step := &steps[index]
		switch actionName(step.Uses) {
		case "step-security/harden-runner":
			indexes.harden = index
		case runsOnActionName:
			indexes.action = index
			indexes.count++
			if !exactRunsOnAction(step) {
				return indexes, []Violation{runsOnViolation(runsOnCanaryJobName, "RunsOn action must use the reviewed release and exact safe inputs")}
			}
		}
		if strings.Contains(step.Run, "./verity ci runs-on verify") {
			indexes.verify = index
		}
		if usesForbiddenRunsOnFeature(step) {
			return indexes, []Violation{runsOnViolation(runsOnCanaryJobName, "persistent or shared cache features are forbidden in the bootstrap canary")}
		}
	}
	return indexes, nil
}

func exactRunsOnAction(step *workflowStep) bool {
	return exactRunsOnActionWithIf(step, "")
}

func exactRunsOnActionWithIf(step *workflowStep, expectedIf string) bool {
	return step.Uses == runsOnActionReference && len(step.With) == 3 && step.With["show_env"] == "false" &&
		step.With["show_costs"] == "summary" && step.With["metrics"] == "cpu,memory,disk,io,network" &&
		len(step.Env) == 0 && !step.ContinueOnError.set &&
		normalizeExpression(step.If) == normalizeExpression(expectedIf)
}

func usesForbiddenRunsOnFeature(step *workflowStep) bool {
	combined := step.Uses + step.Run
	return strings.Contains(combined, "s3-cache") || strings.Contains(combined, "snapshot") ||
		strings.Contains(combined, "tmpfs") || strings.Contains(combined, "efs")
}

func exactRunsOnVerifyArguments(run string) bool {
	expected := []string{
		"./verity", "ci", "runs-on", "verify",
		"--expected-account", `"$EXPECTED_AWS_ACCOUNT"`,
		"--expected-region", `"$EXPECTED_AWS_REGION"`,
		"--expected-arch", "amd64",
		"--minimum-cpu", "4",
		"--minimum-memory-gib", "7",
		"--minimum-disk-gib", "30",
	}
	return slices.Equal(strings.Fields(run), expected)
}

func runsOnViolation(job, detail string) Violation {
	return Violation{Rule: RuleRunsOnBoundary, Workflow: runsOnSmokeWorkflowName, Job: job, Detail: detail}
}
