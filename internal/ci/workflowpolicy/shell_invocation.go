package workflowpolicy

import (
	"path"
	"strings"
)

func parseShellInvocation(command []string) shellInvocation {
	invocation := shellInvocation{executable: -1}
	index := 0
	for index < len(command) && (isShellAssignment(command[index]) || isShellControlWord(command[index])) {
		index++
	}
	for index < len(command) {
		switch {
		case command[index] == "command":
			next, executes := commandExecutableIndex(command, index+1)
			if !executes {
				return invocation
			}
			index = next
		case command[index] == "exec":
			index = execExecutableIndex(command, index+1)
		case path.Base(command[index]) == "env":
			next, directory := envExecutableIndex(command, index+1)
			if next < 0 {
				return invocation
			}
			invocation.workingDirectory = joinShellWorkingDirectory(invocation.workingDirectory, directory)
			index = next
		default:
			invocation.executable = index
			return invocation
		}
	}
	return invocation
}

func commandExecutableIndex(command []string, start int) (int, bool) {
	for index := start; index < len(command); index++ {
		switch command[index] {
		case "-p":
			continue
		case "--":
			return index + 1, index+1 < len(command)
		case "-v", "-V":
			return -1, false
		default:
			if strings.HasPrefix(command[index], "-") {
				return -1, false
			}
			return index, true
		}
	}
	return -1, false
}

func execExecutableIndex(command []string, start int) int {
	for index := start; index < len(command); index++ {
		switch command[index] {
		case "-a":
			index++
		case "-c", "-l":
			continue
		case "--":
			return index + 1
		default:
			return index
		}
	}
	return len(command)
}

func envExecutableIndex(command []string, start int) (executable int, directory string) {
	for index := start; index < len(command); index++ {
		argument := command[index]
		if isShellAssignment(argument) {
			continue
		}
		switch argument {
		case "--":
			return index + 1, directory
		case "-", "-i", "--ignore-environment", "--debug":
			continue
		case "-u", "--unset", "-C", "--chdir", "--argv0":
			if index+1 >= len(command) {
				return -1, directory
			}
			index++
			if argument == "-C" || argument == "--chdir" {
				directory = joinShellWorkingDirectory(directory, command[index])
			}
			continue
		case "-0", "--null", "--help", "--version":
			return -1, directory
		}
		switch {
		case strings.HasPrefix(argument, "--unset="), strings.HasPrefix(argument, "--argv0="):
			continue
		case strings.HasPrefix(argument, "--chdir="):
			next, _ := strings.CutPrefix(argument, "--chdir=")
			directory = joinShellWorkingDirectory(directory, next)
			continue
		case strings.HasPrefix(argument, "-"):
			return -1, directory
		default:
			return index, directory
		}
	}
	return -1, directory
}

func isShellAssignment(value string) bool {
	separator := strings.IndexByte(value, '=')
	if separator <= 0 {
		return false
	}
	for index, character := range value[:separator] {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func isShellControlWord(value string) bool {
	switch value {
	case "if", "then", "elif", "else", "do", "while", "until", "!", "time":
		return true
	default:
		return false
	}
}
