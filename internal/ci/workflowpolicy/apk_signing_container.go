package workflowpolicy

import "strings"

func signerContainerReasons(command []string) []string {
	joined := strings.ToLower(strings.Join(command, " "))
	image, _ := containerImageFromCommand(command[1:])
	if !strings.Contains(strings.ToLower(image), "apk-repository-signer") {
		return nil
	}
	var reasons []string
	if !hasOption(command, "--network", "none") {
		reasons = append(reasons, "signer network must be disabled")
	}
	if !hasNonRootUser(command) {
		reasons = append(reasons, "signer must run as an explicit non-root user")
	}
	if !hasOption(command, "--cap-drop", "all") || strings.Contains(joined, "--cap-add") || strings.Contains(joined, "--privileged") {
		reasons = append(reasons, "signer must drop all capabilities and add none")
	}
	if !containsArgument(command, "--read-only") {
		reasons = append(reasons, "signer root filesystem must be read-only")
	}
	if !hasOption(command, "--security-opt", "no-new-privileges") {
		reasons = append(reasons, "signer must disable privilege escalation")
	}
	if stringContainsAny(joined, " --env ", " --env=", " --env-file ", " --env-file=", " -e ") {
		reasons = append(reasons, "signer container environment injection is forbidden")
	}
	if stringContainsAny(joined, "$github_workspace:", "${github_workspace}:", "${{ github.workspace }}:", "$pwd:", "$(pwd):", "src=$github_workspace", "src=${github_workspace}", "src=${{ github.workspace }}", "src=$pwd", "src=$(pwd)") {
		reasons = append(reasons, "whole-workspace writable mounts are forbidden")
	}
	return reasons
}

func containsArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if strings.EqualFold(argument, wanted) {
			return true
		}
	}
	return false
}

func hasOption(arguments []string, name, value string) bool {
	for index, argument := range arguments {
		if strings.EqualFold(argument, name+"="+value) {
			return true
		}
		if strings.EqualFold(argument, name) && index+1 < len(arguments) && strings.EqualFold(arguments[index+1], value) {
			return true
		}
	}
	return false
}

func hasNonRootUser(arguments []string) bool {
	for index, argument := range arguments {
		value := ""
		lowerArgument := strings.ToLower(argument)
		if user, ok := strings.CutPrefix(lowerArgument, "--user="); ok {
			value = user
		} else if (argument == "--user" || argument == "-u") && index+1 < len(arguments) {
			value = strings.ToLower(arguments[index+1])
		}
		if value != "" {
			return value != "root" && value != "0" && !strings.HasPrefix(value, "0:")
		}
	}
	return false
}
