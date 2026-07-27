package repositoryops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	errInvalidJSONStructure = errors.New("invalid JSON structure")
	trivyReportFields       = map[string]struct{}{
		"SchemaVersion": {}, "Trivy": {}, "ReportID": {}, "CreatedAt": {}, "ArtifactID": {},
		"ArtifactName": {}, "ArtifactType": {}, "Metadata": {}, "Results": {},
	}
	trivyResultFields = map[string]struct{}{
		"Target": {}, "Class": {}, "Type": {}, "Packages": {}, "Vulnerabilities": {},
		"MisconfSummary": {}, "Misconfigurations": {}, "Secrets": {}, "Licenses": {},
		"CustomResources": {}, "ExperimentalModifiedFindings": {},
	}
)

type parsedTrivyResult struct {
	Type               string
	vulnerabilityCount int
}

func parseTrivyResults(data []byte) ([]parsedTrivyResult, error) {
	if err := validateUniqueJSONKeys(data); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedTrivyReport, err)
	}
	fields, err := decodeJSONObject(data, "report")
	if err != nil {
		return nil, err
	}
	if err := rejectUnknownFields(fields, trivyReportFields, "report"); err != nil {
		return nil, err
	}
	resultsJSON, present := fields["Results"]
	if !present || bytes.Equal(resultsJSON, []byte("null")) {
		return nil, fmt.Errorf("%w: Results array is required", ErrMalformedTrivyReport)
	}
	var rawResults []json.RawMessage
	if err := json.Unmarshal(resultsJSON, &rawResults); err != nil {
		return nil, fmt.Errorf("%w: Results must be an array: %w", ErrMalformedTrivyReport, err)
	}
	results := make([]parsedTrivyResult, 0, len(rawResults))
	for index, raw := range rawResults {
		result, err := parseTrivyResult(raw, index)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func parseTrivyResult(data []byte, index int) (parsedTrivyResult, error) {
	label := fmt.Sprintf("Results[%d]", index)
	fields, err := decodeJSONObject(data, label)
	if err != nil {
		return parsedTrivyResult{}, err
	}
	if err := rejectUnknownFields(fields, trivyResultFields, label); err != nil {
		return parsedTrivyResult{}, err
	}
	var result parsedTrivyResult
	if rawType, present := fields["Type"]; present {
		if err := json.Unmarshal(rawType, &result.Type); err != nil {
			return parsedTrivyResult{}, fmt.Errorf("%w: %s Type must be a string: %w", ErrMalformedTrivyReport, label, err)
		}
	}
	rawVulnerabilities, present := fields["Vulnerabilities"]
	if !present || bytes.Equal(rawVulnerabilities, []byte("null")) {
		return result, nil
	}
	var vulnerabilities []json.RawMessage
	if err := json.Unmarshal(rawVulnerabilities, &vulnerabilities); err != nil {
		return parsedTrivyResult{}, fmt.Errorf("%w: %s Vulnerabilities must be an array: %w", ErrMalformedTrivyReport, label, err)
	}
	result.vulnerabilityCount = len(vulnerabilities)
	return result, nil
}

func decodeJSONObject(data []byte, label string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("%w: %s must be an object: %w", ErrMalformedTrivyReport, label, err)
	}
	if fields == nil {
		return nil, fmt.Errorf("%w: %s must be an object", ErrMalformedTrivyReport, label)
	}
	return fields, nil
}

func rejectUnknownFields(fields map[string]json.RawMessage, allowed map[string]struct{}, label string) error {
	for field := range fields {
		if _, present := allowed[field]; !present {
			return fmt.Errorf("%w: %s contains unknown field %q", ErrMalformedTrivyReport, label, field)
		}
	}
	return nil
}

func validateUniqueJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := validateJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("read trailing JSON: %w", err)
		}
		return fmt.Errorf("%w: trailing JSON value", errInvalidJSONStructure)
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON token: %w", err)
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		return validateJSONObjectTokens(decoder)
	case '[':
		return validateJSONArrayTokens(decoder)
	default:
		return fmt.Errorf("%w: unexpected delimiter %q", errInvalidJSONStructure, delimiter)
	}
}

func validateJSONObjectTokens(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("read object key: %w", err)
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("%w: object key is not a string", errInvalidJSONStructure)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%w: duplicate object key %q", errInvalidJSONStructure, key)
		}
		seen[key] = struct{}{}
		if err := validateJSONValue(decoder); err != nil {
			return err
		}
	}
	return consumeJSONDelimiter(decoder, '}')
}

func validateJSONArrayTokens(decoder *json.Decoder) error {
	for decoder.More() {
		if err := validateJSONValue(decoder); err != nil {
			return err
		}
	}
	return consumeJSONDelimiter(decoder, ']')
}

func consumeJSONDelimiter(decoder *json.Decoder, expected json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read closing JSON delimiter: %w", err)
	}
	if token != expected {
		return fmt.Errorf("%w: unexpected closing delimiter %q", errInvalidJSONStructure, token)
	}
	return nil
}
