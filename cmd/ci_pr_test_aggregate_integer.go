package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	prMarkerComponentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,255}$`)
	errTrailingPRJSON        = errors.New("trailing JSON value")
)

func evaluateChangedPRInteger(input *prAggregateInput) error {
	build, err := parsePRExpectedMatrix("build", input.ExpectedIntegerMatrix)
	if err != nil {
		return err
	}
	smoke, err := parsePRExpectedMatrix("smoke", input.ExpectedIntegerSmokeMatrix)
	if err != nil {
		return err
	}
	if err := requirePRSmokeEvidence(input, smoke); err != nil {
		return err
	}
	if len(build.Include) == 0 {
		return fmt.Errorf("%w: expected Integer build matrix must not be empty when changes are present", errPRCommandFailed)
	}
	if err := requirePRSuccess("integer-build-changed", input.IntegerBuildResult); err != nil {
		return err
	}
	return requirePRIntegerMarkers(input.SecurityDir, "build", build.Include)
}

func requirePRSmokeEvidence(input *prAggregateInput, smoke prIntegerExpectedMatrix) error {
	if len(smoke.Include) == 0 {
		if input.IntegerSmokeResult != "skipped" {
			return fmt.Errorf("%w: integer-smoke-test must be skipped: %s", errPRCommandFailed, input.IntegerSmokeResult)
		}
		return nil
	}
	if err := requirePRSuccess("integer-smoke-test", input.IntegerSmokeResult); err != nil {
		return err
	}
	return requirePRIntegerMarkers(input.SecurityDir, "smoke", smoke.Include)
}

func parsePRExpectedMatrix(kind, raw string) (prIntegerExpectedMatrix, error) {
	var matrix prIntegerExpectedMatrix
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&matrix); err != nil || matrix.Include == nil {
		return prIntegerExpectedMatrix{}, fmt.Errorf("%w: invalid expected Integer %s matrix", errPRCommandFailed, kind)
	}
	if err := ensurePRJSONEOF(decoder); err != nil {
		return prIntegerExpectedMatrix{}, fmt.Errorf("%w: invalid expected Integer %s matrix", errPRCommandFailed, kind)
	}
	return matrix, nil
}

func ensurePRJSONEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	return errTrailingPRJSON
}

func requirePRIntegerMarkers(dir, kind string, entries []prIntegerEntry) error {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		key := entry.Image + "\x00" + entry.Version + "\x00" + entry.Type
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		safeImage := strings.NewReplacer("/", "-", ":", "-").Replace(entry.Image)
		if !prMarkerComponentPattern.MatchString(safeImage) || !prMarkerComponentPattern.MatchString(entry.Version) || !prMarkerComponentPattern.MatchString(entry.Type) {
			return fmt.Errorf("%w: invalid Integer %s marker identity", errPRCommandFailed, kind)
		}
		for _, arch := range []string{"amd64", "arm64"} {
			marker := filepath.Join(dir, fmt.Sprintf("%s-%s-%s-%s-%s.passed", kind, safeImage, entry.Version, entry.Type, arch))
			info, err := os.Lstat(marker)
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf(
					"%w: missing successful Integer %s security leg: %s:%s-%s (%s)",
					errPRCommandFailed,
					kind,
					entry.Image,
					entry.Version,
					entry.Type,
					arch,
				)
			}
		}
	}
	return nil
}
