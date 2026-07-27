package workflowpolicy

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrInvalidWorkflow = errors.New("invalid workflow input")
	ErrPolicyViolation = errors.New("workflow policy violation")

	errExpectedScalar        = errors.New("expected YAML scalar")
	errExpectedMapping       = errors.New("expected YAML mapping")
	errMappingScalars        = errors.New("mapping values must be scalars")
	errContainerShape        = errors.New("container must be a scalar or mapping")
	errTriggerListScalars    = errors.New("workflow trigger list values must be scalars")
	errTriggerShape          = errors.New("workflow triggers must be a scalar, sequence, or mapping")
	errNonCanonicalTrigger   = errors.New("noncanonical workflow trigger")
	errUnsupportedTrigger    = errors.New("unsupported workflow trigger")
	errPermissionValue       = errors.New("unsupported permissions value")
	errPermissionShape       = errors.New("permissions must be a mapping, read-all, or write-all")
	errPermissionLevel       = errors.New("unsupported permission level")
	errMultipleYAMLDocuments = errors.New("multiple YAML documents are not allowed")
	errStrictYAMLSchema      = errors.New("strict YAML schema violation")
)

type Rule string

const (
	RulePagesOwner         Rule = "pages-owner"
	RuleAPKPagesPermission Rule = "apk-pages-permission"
	RuleIntegerTriggers    Rule = "integer-triggers"
	RuleProducerIdentity   Rule = "producer-identity"
	RulePinnedReference    Rule = "pinned-reference"
	RuleLeastPrivilege     Rule = "least-privilege"
	RulePRWrite            Rule = "pr-write"
	RulePRPackagesWrite         = RulePRWrite
	RuleGoOwnedLogic       Rule = "go-owned-logic"
	RuleZeroCVEOrdering    Rule = "zero-cve-ordering"
	RuleAPKSigningBoundary Rule = "apk-signing-boundary"
	RuleProtectedDispatch  Rule = "protected-dispatch"
	RulePrivateKeyArtifact Rule = "private-key-artifact"
	RuleSignerProvenance   Rule = "signer-provenance"
)

type Violation struct {
	Rule     Rule
	Workflow string
	Job      string
	Detail   string
}

func (v Violation) String() string {
	location := v.Workflow
	if v.Job != "" {
		location += ":" + v.Job
	}
	return fmt.Sprintf("[%s] %s: %s", v.Rule, location, v.Detail)
}

type PolicyError struct {
	Violations []Violation
}

func (e *PolicyError) Error() string {
	violations := append([]Violation(nil), e.Violations...)
	sort.Slice(violations, func(i, j int) bool {
		left := violations[i]
		right := violations[j]
		if left.Workflow != right.Workflow {
			return left.Workflow < right.Workflow
		}
		if left.Job != right.Job {
			return left.Job < right.Job
		}
		if left.Rule != right.Rule {
			return left.Rule < right.Rule
		}
		return left.Detail < right.Detail
	})

	lines := make([]string, 0, len(violations)+1)
	lines = append(lines, ErrPolicyViolation.Error())
	for _, violation := range violations {
		lines = append(lines, "- "+violation.String())
	}
	return strings.Join(lines, "\n")
}

func (e *PolicyError) Unwrap() error {
	return ErrPolicyViolation
}

type Report struct {
	WorkflowCount int
}
