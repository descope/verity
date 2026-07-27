package workflowpolicy

import (
	"path"
	"strings"
)

const (
	setupVerityPackage = "./.github/actions/setup-verity/cmd/setup-verity"
	verityModulePath   = "github.com/verity-org/verity"
)

func localVerityCompilationReason(location runLocation, command []string) string {
	shell := parseShellInvocation(command)
	if shell.executable < 0 {
		return ""
	}
	location.workingDirectory = joinShellWorkingDirectory(location.workingDirectory, shell.workingDirectory)
	executable := shell.executable
	name := path.Base(command[executable])
	if name == "bash" || name == "sh" {
		return nestedShellCompilationReason(location, command[executable+1:])
	}
	if isSetupVerityExecutable(command[executable]) && nextArgument(command, executable+1) == "build" {
		return "compiled setup-verity helper invoked with the build subcommand"
	}
	if name != "go" {
		return ""
	}
	invocation := parseGoInvocation(command, executable+1)
	if invocation.subcommand < 0 {
		return ""
	}
	location.workingDirectory = joinShellWorkingDirectory(location.workingDirectory, invocation.workingDirectory)
	switch command[invocation.subcommand] {
	case "build":
		return goBuildCompilationReason(location.workingDirectory, command[invocation.subcommand+1:])
	case "install":
		return goInstallCompilationReason(location.workingDirectory, command[invocation.subcommand+1:])
	case "run":
		return goRunCompilationReason(location, command[invocation.subcommand+1:])
	default:
		return ""
	}
}

func nestedShellCompilationReason(location runLocation, arguments []string) string {
	commandString := shellCommandStringIndex(arguments)
	if commandString < 0 {
		return ""
	}
	for _, command := range splitShellCommands(arguments[commandString]) {
		if reason := localVerityCompilationReason(location, command); reason != "" {
			return reason
		}
	}
	return ""
}

func shellCommandStringIndex(arguments []string) int {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return -1
		}
		if argument == "-o" || argument == "+o" || argument == "-O" || argument == "+O" ||
			argument == "--init-file" || argument == "--rcfile" {
			if index+1 >= len(arguments) {
				return -1
			}
			index++
			continue
		}
		if strings.HasPrefix(argument, "--") {
			continue
		}
		if flags, found := strings.CutPrefix(argument, "-"); found {
			if strings.Contains(flags, "c") && index+1 < len(arguments) {
				return index + 1
			}
			continue
		}
		return -1
	}
	return -1
}

func isSetupVerityExecutable(executable string) bool {
	return path.Base(cleanShellPath(executable)) == "setup-verity"
}

func (p *shellCommandParser) carryWorkingDirectory() {
	invocation := parseShellInvocation(p.command)
	if invocation.executable < 0 || invocation.workingDirectory != "" || p.command[invocation.executable] != "cd" {
		return
	}
	index := invocation.executable + 1
	if index < len(p.command) && p.command[index] == "--" {
		index++
	}
	if index+1 == len(p.command) && !strings.HasPrefix(p.command[index], "-") {
		p.workingDirectory = joinShellWorkingDirectory(p.workingDirectory, p.command[index])
	}
}

func joinShellWorkingDirectory(base, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return base
	}
	if base == "" || path.IsAbs(next) || isWorkspacePath(next) {
		return next
	}
	return path.Join(base, next)
}
