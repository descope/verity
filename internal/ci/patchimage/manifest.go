package patchimage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var ErrInvalidPlatform = errors.New("invalid image platform")

type ManifestPlanInput struct {
	ImageName       string
	SourceTag       string
	StagingRegistry string
	Platforms       string
}

type ManifestPlan struct {
	ManifestTag string
	SourceTags  []string
}

func BuildManifestPlan(input ManifestPlanInput) (ManifestPlan, error) {
	safeName := safeImageName(input.ImageName)
	plan := ManifestPlan{ManifestTag: input.StagingRegistry + ":" + safeName + "-" + input.SourceTag}
	for platform := range strings.SplitSeq(input.Platforms, ",") {
		parts := strings.Split(platform, "/")
		if len(parts) < 2 || parts[len(parts)-1] == "" {
			return ManifestPlan{}, fmt.Errorf("%w: %q", ErrInvalidPlatform, platform)
		}
		plan.SourceTags = append(plan.SourceTags, plan.ManifestTag+"-"+parts[len(parts)-1])
	}
	return plan, nil
}

type CompareInput struct {
	PreviousExisted bool
	TargetExists    bool
	Current         TrivySummary
	Previous        TrivySummary
}

func ShouldPublish(input *CompareInput) bool {
	return !input.PreviousExisted ||
		input.Current.VulnerabilityFingerprint() != input.Previous.VulnerabilityFingerprint() ||
		!input.TargetExists
}

var rekorURLPattern = regexp.MustCompile(`https://rekor\.sigstore\.dev/api/v\d+/log/entries\S*`)

func ExtractRekorURL(bundle, output []byte) string {
	if index, ok := findLogIndex(bytes.TrimSpace(bundle)); ok {
		return "https://rekor.sigstore.dev/api/v1/log/entries?logIndex=" + strconv.FormatInt(index, 10)
	}
	return rekorURLPattern.FindString(string(output))
}

func findLogIndex(value json.RawMessage) (int64, bool) {
	if len(value) == 0 {
		return 0, false
	}
	switch value[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(value, &object); err != nil {
			return 0, false
		}
		if raw, exists := object["logIndex"]; exists {
			var index int64
			if err := json.Unmarshal(raw, &index); err == nil {
				return index, true
			}
		}
		for _, raw := range object {
			if index, ok := findLogIndex(raw); ok {
				return index, true
			}
		}
	case '[':
		var array []json.RawMessage
		if err := json.Unmarshal(value, &array); err != nil {
			return 0, false
		}
		for _, raw := range array {
			if index, ok := findLogIndex(raw); ok {
				return index, true
			}
		}
	}
	return 0, false
}
