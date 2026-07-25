package workflowpolicy

import "strings"

var dockerOptionsWithValue = map[string]struct{}{
	"--add-host": {}, "--cap-add": {}, "--cap-drop": {}, "--entrypoint": {},
	"--device": {}, "--env": {}, "--env-file": {}, "--expose": {}, "--hostname": {},
	"--label": {}, "--mount": {}, "--name": {}, "--network": {}, "--platform": {},
	"--publish": {}, "--restart": {}, "--security-opt": {}, "--tmpfs": {},
	"--ulimit": {}, "--user": {}, "--volume": {}, "--workdir": {},
	"-e": {}, "-h": {}, "-l": {}, "-p": {}, "-u": {}, "-v": {}, "-w": {},
}

func inlineContainerImages(run string) []string {
	normalized := strings.ReplaceAll(run, "\\\n", " ")
	var images []string
	for line := range strings.SplitSeq(normalized, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		for index := 0; index < len(fields); index++ {
			if fields[index] != "docker" && fields[index] != "podman" {
				continue
			}
			image, consumed := containerImageFromCommand(fields[index+1:])
			if image != "" {
				images = append(images, strings.Trim(image, "'\""))
			}
			index += consumed
		}
	}
	return images
}

func containerImageFromCommand(arguments []string) (image string, consumed int) {
	commandIndex := 0
	for commandIndex < len(arguments) && strings.HasPrefix(arguments[commandIndex], "-") {
		if _, needsValue := dockerOptionsWithValue[arguments[commandIndex]]; needsValue && commandIndex+1 < len(arguments) {
			commandIndex++
		}
		commandIndex++
	}
	if commandIndex >= len(arguments) {
		return "", len(arguments)
	}
	switch arguments[commandIndex] {
	case "run", "create", "pull":
	default:
		return "", len(arguments)
	}
	for index := commandIndex + 1; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" && index+1 < len(arguments) {
			return arguments[index+1], index + 1
		}
		if strings.HasPrefix(argument, "-") {
			if _, needsValue := dockerOptionsWithValue[argument]; needsValue && index+1 < len(arguments) {
				index++
			}
			continue
		}
		return argument, index
	}
	return "", len(arguments)
}
