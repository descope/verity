package buildmetadata

import (
	"runtime"
	"runtime/debug"
	"sort"
)

type runtimeDetails struct {
	GoVersion   string
	GOOS        string
	GOARCH      string
	CGOEnabled  string
	BuildFlags  []string
	Dirty       *bool
	VCSStatus   VCSStatus
	VCSRevision string
}

func runtimeSettings() runtimeDetails {
	settings := map[string]string{}
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range buildInfo.Settings {
			settings[setting.Key] = setting.Value
		}
	}
	details := runtimeDetailsFromSettings(runtime.GOOS, runtime.GOARCH, settings)
	details.GoVersion = runtime.Version()
	return details
}

func runtimeDetailsFromSettings(goos, goarch string, settings map[string]string) runtimeDetails {
	details := runtimeDetails{
		GOOS: goos, GOARCH: goarch, CGOEnabled: typedCGOValue(settings["CGO_ENABLED"]),
		VCSStatus: UnknownVCSStatus, VCSRevision: UnknownValue,
	}
	switch settings["vcs.modified"] {
	case "true":
		details.Dirty = new(true)
		details.VCSStatus = DirtyVCSStatus
	case "false":
		details.Dirty = new(false)
		details.VCSStatus = CleanVCSStatus
	}
	if isLowerHex(settings["vcs.revision"], 40) {
		details.VCSRevision = settings["vcs.revision"]
	}
	vcsRecorded := settings["vcs"] == "git" || details.VCSRevision != UnknownValue || details.Dirty != nil
	details.BuildFlags = runtimeBuildFlags(settings, vcsRecorded)
	return details
}

func runtimeBuildFlags(settings map[string]string, vcsRecorded bool) []string {
	flags := make([]string, 0, 7)
	if settings["-trimpath"] == "true" {
		flags = append(flags, "-trimpath")
	}
	for _, key := range []string{"-race", "-msan", "-asan"} {
		if settings[key] == "true" {
			flags = append(flags, key)
		}
	}
	if flag := canonicalSettingFlag("-buildmode", settings["-buildmode"]); flag != "" {
		flags = append(flags, flag)
	}
	if flag := canonicalSettingFlag("-compiler", settings["-compiler"]); flag != "" {
		flags = append(flags, flag)
	}
	if vcsRecorded {
		flags = append(flags, "-buildvcs=true")
	}
	sort.Strings(flags)
	return flags
}

func canonicalSettingFlag(key, value string) string {
	flag := key + "=" + value
	if isSafeBuildFlag(flag) {
		return flag
	}
	return ""
}

func typedCGOValue(value string) string {
	if value == "0" || value == "1" {
		return value
	}
	return UnknownValue
}
