package patchimage

import (
	"strings"
	"time"
)

type WorkflowInputs struct {
	SourceRef      string
	ImageName      string
	TargetRegistry string
}

type ParsedInputs struct {
	SourceTag       string
	SafeName        string
	StagingRegistry string
}

func ParseInputs(input WorkflowInputs) ParsedInputs {
	withoutDigest := strings.SplitN(input.SourceRef, "@", 2)[0]
	sourceTag := withoutDigest
	if separator := strings.LastIndexByte(withoutDigest, ':'); separator >= 0 {
		sourceTag = withoutDigest[separator+1:]
	}
	return ParsedInputs{
		SourceTag:       sourceTag,
		SafeName:        safeImageName(input.ImageName),
		StagingRegistry: input.TargetRegistry + "/cache",
	}
}

func PlatformRequested(platforms, candidate string) bool {
	return strings.Contains(platforms, candidate)
}

func TrivyDateKey(instant time.Time) string {
	return instant.UTC().Format("2006-01-02-15")
}

func safeImageName(value string) string {
	return strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(value)
}
