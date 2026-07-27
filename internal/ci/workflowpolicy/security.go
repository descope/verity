package workflowpolicy

import (
	"fmt"
	"slices"
	"strings"
)

const (
	pagesScope        permissionScope = "pages"
	idTokenScope      permissionScope = "id-token"
	packagesScope     permissionScope = "packages"
	contentsScope     permissionScope = "contents"
	actionsScope      permissionScope = "actions"
	attestationsScope permissionScope = "attestations"
	securityScope     permissionScope = "security-events"
	pullRequestsScope permissionScope = "pull-requests"
	issuesScope       permissionScope = "issues"
)

var writePermissionMarkers = map[permissionScope][]string{
	pagesScope:        {"actions/deploy-pages@"},
	idTokenScope:      {"actions/deploy-pages@", "actions/attest-build-provenance@", "cosign sign", "patch-image.yaml"},
	packagesScope:     {"crane copy", "docker push", "apko publish", "oras push", "patch-image.yaml", "patch-image.sh", "chart-gen.yaml", "./verity chart-gen", "./verity ci integer-image publish"},
	contentsScope:     {"git push", "gh release", "create-pull-request", "patch-image.yaml", "push-reports.sh", "metrics-finalize.yaml"},
	actionsScope:      {"gh workflow run", "gh run cancel"},
	attestationsScope: {"actions/attest-build-provenance@", "actions/attest@", "gh attestation", "patch-image.yaml"},
	securityScope:     {"github/codeql-action/upload-sarif@"},
	pullRequestsScope: {"gh pr ", "create-pull-request"},
	issuesScope:       {"gh issue "},
}

type deployer struct {
	workflow string
	job      string
}

func validatePagesOwnership(workflows []workflowFile) []Violation {
	var deployers []deployer
	var violations []Violation
	for index := range workflows {
		file := &workflows[index]
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				if actionName(step.Uses) == "actions/deploy-pages" {
					deployers = append(deployers, deployer{workflow: file.Name, job: jobName})
				}
			}
		}
	}
	if len(deployers) != 1 {
		location := "build-site.yaml"
		job := ""
		for _, candidate := range deployers {
			if candidate.workflow != "build-site.yaml" {
				location = candidate.workflow
				job = candidate.job
				break
			}
		}
		violations = append(violations, Violation{
			Rule: RulePagesOwner, Workflow: location, Job: job,
			Detail: fmt.Sprintf("expected exactly one actions/deploy-pages owner in build-site.yaml, found %d", len(deployers)),
		})
	} else if deployers[0].workflow != "build-site.yaml" {
		violations = append(violations, Violation{
			Rule: RulePagesOwner, Workflow: deployers[0].workflow, Job: deployers[0].job,
			Detail: "actions/deploy-pages is owned exclusively by build-site.yaml",
		})
	}

	if apk, ok := findWorkflow(workflows, "apk-repository.yaml"); ok {
		if apk.Workflow.Permissions.level(pagesScope) != permissionNone {
			violations = append(violations, Violation{
				Rule: RuleAPKPagesPermission, Workflow: apk.Name,
				Detail: "APK repository workflow must not declare Pages permissions",
			})
		}
		for _, jobName := range sortedJobNames(apk.Workflow.Jobs) {
			job := apk.Workflow.Jobs[jobName]
			if effectivePermission(apk.Workflow.Permissions, job.Permissions, pagesScope) != permissionNone {
				violations = append(violations, Violation{
					Rule: RuleAPKPagesPermission, Workflow: apk.Name, Job: jobName,
					Detail: "APK repository jobs must not receive Pages permissions",
				})
			}
		}
	}
	return violations
}

func validatePermissions(workflows []workflowFile) []Violation {
	var violations []Violation
	for index := range workflows {
		file := &workflows[index]
		if !file.Workflow.Permissions.declared {
			violations = append(violations, Violation{
				Rule: RuleLeastPrivilege, Workflow: file.Name,
				Detail: "workflow permissions must be explicit",
			})
		}
		violations = append(violations, validatePRWrites(file)...)
		if file.Workflow.Permissions.all == permissionWrite {
			violations = append(violations, Violation{
				Rule: RuleLeastPrivilege, Workflow: file.Name,
				Detail: "workflow-level write-all is forbidden; grant writes only to the consuming job",
			})
		}
		for scope, level := range file.Workflow.Permissions.scopes {
			if level == permissionWrite {
				violations = append(violations, Violation{
					Rule: RuleLeastPrivilege, Workflow: file.Name,
					Detail: fmt.Sprintf("workflow-level %s: write must be scoped to a consuming job", scope),
				})
			}
		}

		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			identity := workflowJobIdentity{workflow: file.Name, job: jobName}
			if !job.Permissions.declared {
				violations = append(violations, Violation{
					Rule: RuleLeastPrivilege, Workflow: file.Name, Job: jobName,
					Detail: "job permissions must be explicit",
				})
			}
			for _, scope := range job.Permissions.writeScopes() {
				usesWrite := jobUsesWritePermission(&job, scope)
				if _, local := localReusableWorkflowName(job.Uses); local {
					usesWrite = reusableJobUsesWritePermission(workflows, identity, &job, scope)
				}
				if !usesWrite {
					violations = append(violations, Violation{
						Rule: RuleLeastPrivilege, Workflow: file.Name, Job: jobName,
						Detail: fmt.Sprintf("%s: write is not used by this job", scope),
					})
				}
			}
		}
	}
	return violations
}

func effectivePermission(workflowPermissions, jobPermissions permissions, scope permissionScope) permissionLevel {
	if jobPermissions.declared {
		return jobPermissions.level(scope)
	}
	return workflowPermissions.level(scope)
}

func (p permissions) writeScopes() []permissionScope {
	if p.all == permissionWrite {
		return []permissionScope{"write-all"}
	}
	result := make([]permissionScope, 0, len(p.scopes))
	for scope, level := range p.scopes {
		if level == permissionWrite {
			result = append(result, scope)
		}
	}
	slices.Sort(result)
	return result
}

func jobUsesWritePermission(job *workflowJob, scope permissionScope) bool {
	if jobUsesTypedWritePermission(job, scope) {
		return true
	}
	var text strings.Builder
	text.WriteString(strings.ToLower(job.Uses))
	for stepIndex := range job.Steps {
		step := &job.Steps[stepIndex]
		text.WriteByte('\n')
		text.WriteString(strings.ToLower(step.Uses))
		text.WriteByte('\n')
		text.WriteString(strings.ToLower(step.Run))
	}
	jobText := text.String()
	for _, marker := range writePermissionMarkers[scope] {
		if strings.Contains(jobText, marker) {
			return true
		}
	}
	return false
}
