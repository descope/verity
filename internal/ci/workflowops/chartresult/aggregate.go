package chartresult

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	positiveIntegerPattern = regexp.MustCompile(`^[1-9]\d*$`)
	sourceSHAPattern       = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)
	identityTokenPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	artifactDigestPattern  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

var standardResults = map[string]map[string]struct{}{
	"discover-charts": {"success": {}},
	"chart-test":      {"success": {}, "skipped": {}},
}

var privilegedResults = map[string]map[string]struct{}{
	"chart-test": {"success": {}},
}

func Aggregate(input *Input) (Result, error) {
	if input == nil {
		return Result{}, fmt.Errorf("%w: input is required", ErrInvalidResults)
	}
	expected, err := expectedResults(input.Profile)
	if err != nil {
		return Result{}, err
	}
	if err := parseResults(input.Results, expected); err != nil {
		return Result{}, err
	}
	return parseIdentity(&input.Identity)
}

func expectedResults(profile string) (map[string]map[string]struct{}, error) {
	switch profile {
	case "", "standard":
		return standardResults, nil
	case "privileged":
		return privilegedResults, nil
	default:
		return nil, fmt.Errorf("%w: unknown profile %q", ErrInvalidResults, profile)
	}
}

func parseResults(values []string, expectedResults map[string]map[string]struct{}) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name, result, found := strings.Cut(value, "=")
		allowed, expected := expectedResults[name]
		_, validResult := allowed[result]
		if !found || strings.Contains(result, "=") || !expected || !validResult {
			return fmt.Errorf("%w: result %q", ErrInvalidResults, value)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("%w: duplicate result %q", ErrInvalidResults, name)
		}
		seen[name] = struct{}{}
	}
	for name := range expectedResults {
		if _, found := seen[name]; !found {
			return fmt.Errorf("%w: missing result %q", ErrInvalidResults, name)
		}
	}
	return nil
}

func parseIdentity(input *IdentityInput) (Result, error) {
	values := []string{
		input.SourceSHA, input.RunID, input.RunAttempt, input.PublicationID,
		input.BatchID, input.ArtifactName, input.ArtifactDigest,
	}
	populated := 0
	for _, value := range values {
		if value != "" {
			populated++
		}
	}
	if populated == 0 {
		return Result{}, nil
	}
	if populated != len(values) || !sourceSHAPattern.MatchString(input.SourceSHA) ||
		!validPositiveInteger(input.RunID) || !validPositiveInteger(input.RunAttempt) ||
		!identityTokenPattern.MatchString(input.PublicationID) || !identityTokenPattern.MatchString(input.BatchID) ||
		!identityTokenPattern.MatchString(input.ArtifactName) || !artifactDigestPattern.MatchString(input.ArtifactDigest) {
		return Result{}, fmt.Errorf("%w: exact producer fields are incomplete or malformed", ErrInvalidIdentity)
	}
	return Result(*input), nil
}

func validPositiveInteger(value string) bool {
	if !positiveIntegerPattern.MatchString(value) {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}
