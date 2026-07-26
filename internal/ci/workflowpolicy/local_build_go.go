package workflowpolicy

import (
	"path"
	"strings"
)

type goInvocation struct {
	subcommand       int
	workingDirectory string
}

func parseGoInvocation(command []string, start int) goInvocation {
	invocation := goInvocation{subcommand: -1}
	for index := start; index < len(command); index++ {
		argument := command[index]
		if argument == "-C" {
			if index+1 >= len(command) {
				return invocation
			}
			index++
			invocation.workingDirectory = command[index]
			continue
		}
		if directory, found := strings.CutPrefix(argument, "-C="); found {
			invocation.workingDirectory = directory
			continue
		}
		if strings.HasPrefix(argument, "-") {
			continue
		}
		if argument == "build" || argument == "install" || argument == "run" {
			invocation.subcommand = index
		}
		return invocation
	}
	return invocation
}

func goBuildCompilationReason(workingDirectory string, arguments []string) string {
	output, packages := parseGoBuildArguments(arguments)
	if writesWorkspaceVerity(output) {
		return "go build writes a Verity binary"
	}
	if len(packages) == 0 && isRepositoryRootDirectory(workingDirectory) {
		return "go build compiles the repository root"
	}
	for _, target := range packages {
		if isVerityRootTarget(workingDirectory, target) {
			return "go build compiles the repository root"
		}
		if isSetupVerityTarget(target) {
			return "go build compiles the setup-verity build helper"
		}
	}
	return ""
}

func goInstallCompilationReason(workingDirectory string, arguments []string) string {
	_, packages := parseGoBuildArguments(arguments)
	for _, target := range packages {
		if isVerityRootTarget(workingDirectory, target) {
			return "go install compiles the Verity repository"
		}
		if isSetupVerityTarget(target) {
			return "go install compiles the setup-verity build helper"
		}
	}
	return ""
}

func parseGoBuildArguments(arguments []string) (output string, packages []string) {
	packages = make([]string, 0, 2)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "-o" && index+1 < len(arguments) {
			index++
			output = arguments[index]
			continue
		}
		if value, found := strings.CutPrefix(argument, "-o="); found {
			output = value
			continue
		}
		if strings.HasPrefix(argument, "-") {
			if goBuildFlagTakesValue(argument) && index+1 < len(arguments) {
				index++
			}
			continue
		}
		packages = append(packages, argument)
	}
	return output, packages
}

func goBuildFlagTakesValue(argument string) bool {
	if strings.Contains(argument, "=") {
		return false
	}
	switch argument {
	case "-asmflags", "-buildmode", "-compiler", "-coverpkg", "-gccgoflags", "-gcflags", "-installsuffix", "-ldflags", "-mod", "-modfile", "-overlay", "-p", "-pgo", "-pkgdir", "-tags", "-toolexec":
		return true
	default:
		return false
	}
}

func goRunCompilationReason(location runLocation, arguments []string) string {
	targetIndex := firstGoRunTarget(arguments)
	if targetIndex < 0 {
		return ""
	}
	target := arguments[targetIndex]
	if isVerityRootTarget(location.workingDirectory, target) {
		return "go run compiles the repository root"
	}
	if !isSetupVerityTarget(target) || nextArgument(arguments, targetIndex+1) != "build" {
		return ""
	}
	if location.isWorkflow && (location.file == buildVerityWorkflowName || location.file == protectedBuildVerityWorkflowName) && location.job == "build" &&
		location.stepID == "build" && target == setupVerityPackage {
		return ""
	}
	return "setup-verity build helper invoked outside build-verity.yaml"
}

func firstGoRunTarget(arguments []string) int {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if strings.HasPrefix(argument, "-") {
			if goRunFlagTakesValue(argument) && index+1 < len(arguments) {
				index++
			}
			continue
		}
		return index
	}
	return -1
}

func goRunFlagTakesValue(argument string) bool {
	if strings.Contains(argument, "=") {
		return false
	}
	switch argument {
	case "-C", "-exec", "-mod", "-modfile", "-overlay", "-pgo", "-tags":
		return true
	default:
		return false
	}
}

func nextArgument(arguments []string, start int) string {
	for index := start; index < len(arguments); index++ {
		if !strings.HasPrefix(arguments[index], "-") {
			return arguments[index]
		}
	}
	return ""
}

func writesWorkspaceVerity(output string) bool {
	return output != "" && path.Base(strings.TrimSpace(output)) == "verity"
}

func isVerityRootTarget(workingDirectory, target string) bool {
	target = strings.TrimSpace(target)
	versionless := target
	if separator := strings.LastIndexByte(versionless, '@'); separator >= 0 {
		versionless = versionless[:separator]
	}
	if versionless == verityModulePath || versionless == verityModulePath+"/..." {
		return true
	}

	directory, knownDirectory := repositoryRelativeDirectory(workingDirectory)
	targetPath, knownTarget := repositoryRelativePath(versionless)
	if !knownTarget {
		return false
	}
	if !knownDirectory {
		if isExternalWorkingDirectory(workingDirectory) && !isWorkspacePath(versionless) {
			return false
		}
		return targetPath == "." || targetPath == "main.go" || targetPath == "*.go" || targetPath == "..."
	}
	if base, recursive := strings.CutSuffix(targetPath, "/..."); recursive {
		return path.Clean(path.Join(directory, base)) == "."
	}
	resolved := path.Clean(path.Join(directory, targetPath))
	return resolved == "." || resolved == "main.go" || resolved == "*.go"
}

func isRepositoryRootDirectory(directory string) bool {
	relative, known := repositoryRelativeDirectory(directory)
	if known {
		return relative == "."
	}
	return !isExternalWorkingDirectory(directory)
}

func repositoryRelativeDirectory(directory string) (string, bool) {
	if strings.TrimSpace(directory) == "" {
		return ".", true
	}
	return repositoryRelativePath(directory)
}

func repositoryRelativePath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	for _, workspace := range []string{"$GITHUB_WORKSPACE", "${GITHUB_WORKSPACE}", "${{ github.workspace }}"} {
		if value == workspace {
			return ".", true
		}
		if relative, found := strings.CutPrefix(value, workspace+"/"); found {
			return path.Clean(relative), true
		}
	}
	if path.IsAbs(value) || strings.Contains(value, "$") {
		return "", false
	}
	return path.Clean(value), true
}

func isWorkspacePath(value string) bool {
	for _, workspace := range []string{"$GITHUB_WORKSPACE", "${GITHUB_WORKSPACE}", "${{ github.workspace }}"} {
		if value == workspace || strings.HasPrefix(value, workspace+"/") {
			return true
		}
	}
	return false
}

func isExternalWorkingDirectory(value string) bool {
	value = strings.TrimSpace(value)
	if path.IsAbs(value) {
		return true
	}
	for _, root := range []string{"$RUNNER_TEMP", "${RUNNER_TEMP}", "${{ runner.temp }}"} {
		if value == root || strings.HasPrefix(value, root+"/") {
			return true
		}
	}
	return false
}

func isSetupVerityTarget(target string) bool {
	cleaned := cleanShellPath(target)
	return cleaned == cleanShellPath(setupVerityPackage) || cleaned == verityModulePath+"/.github/actions/setup-verity/cmd/setup-verity"
}
