package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
)

type copaChangeMode string

const (
	copaChangeModeAll    copaChangeMode = "all"
	copaChangeModeFilter copaChangeMode = "filter"
)

type copaChangeRequest struct {
	repository string
	baseSHA    string
	headSHA    string
	configPath string
}

type copaChangePlan struct {
	mode  copaChangeMode
	names []string
}

var nightlyCopaChangesCmd = &cli.Command{
	Name:  "detect",
	Usage: "Detect semantically added or modified COPA image definitions",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "repository", Value: "."},
		&cli.StringFlag{Name: "base-sha", Required: true},
		&cli.StringFlag{Name: "head-sha", Required: true},
		&cli.StringFlag{Name: "config", Value: "copa-config.yaml"},
		&cli.StringFlag{Name: "github-output", Required: true},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		plan, err := detectCopaChanges(ctx, copaChangeRequest{
			repository: command.String("repository"),
			baseSHA:    command.String("base-sha"),
			headSHA:    command.String("head-sha"),
			configPath: command.String("config"),
		})
		if err != nil {
			return err
		}
		if err := appendCopaChangeOutput(command.String("github-output"), plan); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "COPA change mode: %s", plan.mode)
		if len(plan.names) > 0 {
			fmt.Fprintf(os.Stderr, " (%s)", strings.Join(plan.names, ","))
		}
		fmt.Fprintln(os.Stderr)
		return nil
	},
}

func detectCopaChanges(ctx context.Context, request copaChangeRequest) (copaChangePlan, error) {
	configPath := filepath.ToSlash(request.configPath)
	configChanged := strings.Trim(request.baseSHA, "0") == ""
	if !configChanged {
		output, err := runCopaGitCommand(ctx, request.repository, "diff", "--name-only", request.baseSHA, request.headSHA, "--")
		if err != nil {
			return copaChangePlan{}, err
		}
		for path := range strings.SplitSeq(string(output), "\n") {
			if path == configPath {
				configChanged = true
				break
			}
		}
	}
	if !configChanged {
		return copaChangePlan{mode: copaChangeModeAll}, nil
	}

	headData, err := os.ReadFile(filepath.Join(request.repository, filepath.FromSlash(configPath)))
	if err != nil {
		return copaChangePlan{}, fmt.Errorf("reading current COPA config: %w", err)
	}
	headConfig, err := parseCopaConfig(headData)
	if err != nil {
		return copaChangePlan{}, fmt.Errorf("parsing current COPA config: %w", err)
	}

	var baseConfig config.CopaConfig
	baseData, showErr := runCopaGitCommand(ctx, request.repository, "show", request.baseSHA+":"+configPath)
	if showErr == nil {
		if parsed, parseErr := parseCopaConfig(baseData); parseErr == nil {
			baseConfig = parsed
		}
	}
	names := changedCopaImageNames(baseConfig.Images, headConfig.Images)
	if len(names) == 0 {
		return copaChangePlan{mode: copaChangeModeAll}, nil
	}
	return copaChangePlan{mode: copaChangeModeFilter, names: names}, nil
}

func parseCopaConfig(data []byte) (config.CopaConfig, error) {
	var parsed config.CopaConfig
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return config.CopaConfig{}, fmt.Errorf("decoding YAML: %w", err)
	}
	return parsed, nil
}

func changedCopaImageNames(base, head []config.ImageSpec) []string {
	baseByName := groupCopaImages(base)
	headByName := groupCopaImages(head)
	names := make([]string, 0, len(headByName))
	for name, specs := range headByName {
		if !reflect.DeepEqual(baseByName[name], specs) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func groupCopaImages(images []config.ImageSpec) map[string][]config.ImageSpec {
	grouped := make(map[string][]config.ImageSpec, len(images))
	for index := range images {
		image := &images[index]
		if image.Name != "" {
			grouped[image.Name] = append(grouped[image.Name], *image)
		}
	}
	return grouped
}

func runCopaGitCommand(ctx context.Context, repository string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func appendCopaChangeOutput(path string, plan copaChangePlan) (returnErr error) {
	output, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening GitHub output %s: %w", path, err)
	}
	defer func() {
		if closeErr := output.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("closing GitHub output %s: %w", path, closeErr))
		}
	}()
	if _, err := fmt.Fprintf(output, "mode=%s\n", plan.mode); err != nil {
		return fmt.Errorf("writing GitHub output %s: %w", path, err)
	}
	if len(plan.names) > 0 {
		if _, err := fmt.Fprintf(output, "filter=%s\n", strings.Join(plan.names, ",")); err != nil {
			return fmt.Errorf("writing GitHub output %s: %w", path, err)
		}
	}
	return nil
}
