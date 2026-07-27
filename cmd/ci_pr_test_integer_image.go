package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var prSHA256DigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type prIntegerCommandRunner interface {
	Run(context.Context, *prCommandRequest) (prCommandResult, error)
}

type execPRIntegerCommandRunner struct{}

func (execPRIntegerCommandRunner) Run(ctx context.Context, request *prCommandRequest) (prCommandResult, error) {
	return requirePRCommand(ctx, request)
}

type prIntegerLoadRequest struct {
	TarPath      string
	Architecture string
}

type prIntegerLoadedImage struct {
	Reference    string
	Architecture string
	User         string
}

func loadPRIntegerImage(
	ctx context.Context,
	runner prIntegerCommandRunner,
	request prIntegerLoadRequest,
) (prIntegerLoadedImage, error) {
	if runner == nil || (request.Architecture != "amd64" && request.Architecture != "arm64") {
		return prIntegerLoadedImage{}, fmt.Errorf("%w: invalid Integer image load request", errPRCommandFailed)
	}
	loaded, err := runner.Run(ctx, &prCommandRequest{
		Name: "docker", Args: []string{"load", "--input", request.TarPath},
	})
	if err != nil {
		return prIntegerLoadedImage{}, fmt.Errorf("load Integer image: %w", err)
	}
	reference, err := parsePRDockerLoadReference(string(loaded.Stdout) + "\n" + string(loaded.Stderr))
	if err != nil {
		return prIntegerLoadedImage{}, err
	}
	inspected, err := runner.Run(ctx, &prCommandRequest{
		Name: "docker", Args: []string{"image", "inspect", reference},
	})
	if err != nil {
		return prIntegerLoadedImage{}, fmt.Errorf("inspect loaded Integer image: %w", err)
	}
	var images []struct {
		ID           string `json:"Id"`
		Architecture string `json:"Architecture"`
		Config       struct {
			User string `json:"User"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(inspected.Stdout, &images); err != nil || len(images) != 1 {
		return prIntegerLoadedImage{}, fmt.Errorf("%w: malformed docker image inspection", errPRCommandFailed)
	}
	image := images[0]
	if !prSHA256DigestPattern.MatchString(image.ID) {
		return prIntegerLoadedImage{}, fmt.Errorf("%w: loaded image has malformed ID %q", errPRCommandFailed, image.ID)
	}
	if image.Architecture != request.Architecture {
		return prIntegerLoadedImage{}, fmt.Errorf(
			"%w: runtime architecture mismatch: expected %s, got %s",
			errPRCommandFailed,
			request.Architecture,
			image.Architecture,
		)
	}
	return prIntegerLoadedImage{Reference: image.ID, Architecture: image.Architecture, User: image.Config.User}, nil
}

func parsePRDockerLoadReference(output string) (string, error) {
	reference := ""
	for line := range strings.SplitSeq(output, "\n") {
		for _, prefix := range []string{"Loaded image: ", "Loaded image ID: "} {
			if value, present := strings.CutPrefix(line, prefix); present {
				reference = strings.TrimSpace(value)
			}
		}
	}
	if reference == "" || strings.ContainsAny(reference, "\r\n\x00") {
		return "", fmt.Errorf("%w: docker load did not report an image reference", errPRCommandFailed)
	}
	return reference, nil
}
