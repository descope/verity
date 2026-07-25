package buildmetadata

import (
	"fmt"
	"sort"
	"strings"
)

const (
	maxVersionLength      = 64
	maxInjectedBuildFlags = 8
)

type resolvedIdentity struct {
	Version     string
	SourceSHA   string
	BuildKey    string
	BuildStatus BuildStatus
}

func resolveCurrent(details *runtimeDetails) (Info, error) {
	resolvedVersion, release, err := parseInjectedVersion(version)
	if err != nil {
		return Info{}, err
	}
	resolvedSourceSHA, err := parseInjectedDigest(sourceSHA, 40, "source SHA")
	if err != nil {
		return Info{}, err
	}
	resolvedBuildKey, err := parseInjectedDigest(buildKey, 64, "build key")
	if err != nil {
		return Info{}, err
	}
	if err := validateInjectedBuildFlags(buildFlags, details.BuildFlags); err != nil {
		return Info{}, err
	}
	if resolvedSourceSHA != UnknownValue && details.VCSRevision != UnknownValue && resolvedSourceSHA != details.VCSRevision {
		return Info{}, fmt.Errorf("%w: source revision mismatch", ErrInvalidMetadata)
	}

	status := DevelopmentStatus
	if release && resolvedSourceSHA != UnknownValue && resolvedBuildKey != UnknownValue {
		status = BuiltStatus
	}
	return infoFromRuntime(resolvedIdentity{
		Version: resolvedVersion, SourceSHA: resolvedSourceSHA, BuildKey: resolvedBuildKey,
		BuildStatus: status,
	}, details), nil
}

func parseInjectedVersion(raw string) (versionValue string, release bool, err error) {
	if raw == DevelopmentVersion {
		return DevelopmentVersion, false, nil
	}
	if raw == "" || len(raw) > maxVersionLength || isVersionPlaceholder(raw) || !isVersionToken(raw) {
		return "", false, fmt.Errorf("%w: malformed version", ErrInvalidMetadata)
	}
	return raw, true, nil
}

func isVersionPlaceholder(value string) bool {
	switch strings.ToLower(value) {
	case DevelopmentVersion, UnknownValue, "development", "unset", "none":
		return true
	default:
		return false
	}
}

func isVersionToken(value string) bool {
	if value == "" || !isVersionAlphaNumeric(value[0]) || !isVersionAlphaNumeric(value[len(value)-1]) {
		return false
	}
	for index := 1; index < len(value)-1; index++ {
		character := value[index]
		if !isVersionAlphaNumeric(character) && character != '.' && character != '_' && character != '+' && character != '-' {
			return false
		}
	}
	return true
}

func isVersionAlphaNumeric(character byte) bool {
	return character >= '0' && character <= '9' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func parseInjectedDigest(raw string, length int, name string) (string, error) {
	if raw == "" || raw == UnknownValue {
		return UnknownValue, nil
	}
	if !isLowerHex(raw, length) {
		return "", fmt.Errorf("%w: malformed %s", ErrInvalidMetadata, name)
	}
	return raw, nil
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validateInjectedBuildFlags(raw string, actual []string) error {
	if raw == "" {
		return nil
	}
	tokens := strings.Fields(raw)
	if len(tokens) == 0 || len(tokens) > maxInjectedBuildFlags {
		return fmt.Errorf("%w: invalid build flags", ErrInvalidMetadata)
	}
	flags, err := canonicalBuildFlags(tokens)
	if err != nil {
		return err
	}
	recorded := make(map[string]struct{}, len(actual))
	for _, flag := range actual {
		recorded[flag] = struct{}{}
	}
	for _, flag := range flags {
		if _, ok := recorded[flag]; !ok {
			return fmt.Errorf("%w: build flags mismatch", ErrInvalidMetadata)
		}
	}
	return nil
}

func canonicalBuildFlags(flags []string) ([]string, error) {
	canonical := make([]string, 0, len(flags))
	seen := make(map[string]struct{}, len(flags))
	for _, flag := range flags {
		if !isSafeBuildFlag(flag) {
			return nil, fmt.Errorf("%w: invalid build flags", ErrInvalidMetadata)
		}
		if _, ok := seen[flag]; ok {
			continue
		}
		seen[flag] = struct{}{}
		canonical = append(canonical, flag)
	}
	sort.Strings(canonical)
	return canonical, nil
}

func isSafeBuildFlag(flag string) bool {
	switch flag {
	case "-trimpath", "-race", "-msan", "-asan", "-buildvcs=true",
		"-buildmode=archive", "-buildmode=c-archive", "-buildmode=c-shared",
		"-buildmode=default", "-buildmode=exe", "-buildmode=pie",
		"-buildmode=plugin", "-buildmode=shared", "-compiler=gc", "-compiler=gccgo":
		return true
	default:
		return false
	}
}
