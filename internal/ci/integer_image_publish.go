package ci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	integerImageNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:/[a-z][a-z0-9-]*)*$`)
	integerImageValuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	integerRegistryPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9./:_-]*$`)
)

type IntegerImagePublishOptions struct {
	Image         string
	Version       string
	Type          string
	Registry      string
	Tags          string
	Title         string
	Description   string
	ConfigPath    string
	Workspace     string
	SourceSHA     string
	RunID         uint64
	RunAttempt    uint64
	PublicationID string
	Melange       bool
	CreatedAt     time.Time
	Runner        IntegerImageRunner
}

type IntegerImageCommand struct {
	Name          string
	Args          []string
	Dir           string
	CaptureOutput bool
}

func (command IntegerImageCommand) ID() string {
	if len(command.Args) == 0 {
		return command.Name
	}
	return command.Name + ":" + command.Args[0]
}

type IntegerImageRunner interface {
	Run(context.Context, IntegerImageCommand) ([]byte, error)
}

func PublishIntegerImage(ctx context.Context, options *IntegerImagePublishOptions) (digest string, err error) {
	tags, err := validateIntegerImagePublishOptions(options)
	if err != nil {
		return "", err
	}
	configPath, err := prepareIntegerPublishConfig(options)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, os.Remove(configPath)) }()

	runner := options.Runner
	if runner == nil {
		runner = execIntegerImageRunner{}
	}
	stagingRef := fmt.Sprintf(
		"%s/cache/%s:%s-%s-%d-%d", options.Registry, options.Image, tags[0], options.PublicationID, options.RunID, options.RunAttempt,
	)
	apkoArgs := []string{"publish", "--arch", "amd64,arm64", "--sbom-path", options.Workspace}
	if options.Melange {
		apkoArgs = append(
			apkoArgs,
			"--repository-append", "@local "+filepath.Join(options.Workspace, "packages", "repo"),
			"--keyring-append", filepath.Join(options.Workspace, "packages", "repo", "x86_64", "melange.rsa.pub"),
			"--keyring-append", filepath.Join(options.Workspace, "packages", "repo", "aarch64", "melange.rsa.pub"),
		)
	}
	apkoArgs = append(apkoArgs, configPath, stagingRef)
	if _, err := runner.Run(ctx, IntegerImageCommand{Name: "apko", Args: apkoArgs}); err != nil {
		return "", fmt.Errorf("stage Integer image: %w", err)
	}
	trivyArgs := []string{
		"image", "--exit-code", "1", "--severity", "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL",
		"--vuln-type", "os,library", "--format", "json", "--output", "trivy-report.json", stagingRef,
	}
	if _, err := runner.Run(ctx, IntegerImageCommand{Name: "trivy", Args: trivyArgs}); err != nil {
		return "", fmt.Errorf("staged Integer image failed zero-CVE gate: %w", err)
	}
	for _, tag := range tags {
		finalRef := options.Registry + "/" + options.Image + ":" + tag
		if _, err := runner.Run(ctx, IntegerImageCommand{Name: "crane", Args: []string{"copy", stagingRef, finalRef}}); err != nil {
			return "", fmt.Errorf("promote Integer image %s: %w", finalRef, err)
		}
	}
	primaryRef := options.Registry + "/" + options.Image + ":" + tags[0]
	output, err := runner.Run(ctx, IntegerImageCommand{Name: "crane", Args: []string{"digest", primaryRef}, CaptureOutput: true})
	if err != nil {
		return "", fmt.Errorf("read promoted Integer digest: %w", err)
	}
	digest = strings.TrimSpace(string(output))
	if !integerDigestPattern.MatchString(digest) {
		return "", fmt.Errorf("%w: invalid promoted image digest", ErrIntegerBatchPlan)
	}
	return digest, nil
}

func validateIntegerImagePublishOptions(options *IntegerImagePublishOptions) ([]string, error) {
	if options == nil || !integerImageNamePattern.MatchString(options.Image) ||
		!integerImageValuePattern.MatchString(options.Version) || !integerImageValuePattern.MatchString(options.Type) ||
		!integerRegistryPattern.MatchString(options.Registry) || options.ConfigPath == "" || options.Workspace == "" ||
		!integerSourceSHAPattern.MatchString(options.SourceSHA) || options.RunID == 0 || options.RunAttempt == 0 ||
		!integerPublicationIDPattern.MatchString(options.PublicationID) || options.CreatedAt.IsZero() {
		return nil, fmt.Errorf("%w: image publication options", ErrIntegerBatchPlan)
	}
	rawTags := strings.Split(options.Tags, ",")
	tags := make([]string, 0, len(rawTags))
	for _, tag := range rawTags {
		if tag = strings.TrimSpace(tag); !integerImageValuePattern.MatchString(tag) {
			return nil, fmt.Errorf("%w: invalid image tag %q", ErrIntegerBatchPlan, tag)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

func prepareIntegerPublishConfig(options *IntegerImagePublishOptions) (path string, err error) {
	data, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		return "", fmt.Errorf("read generated apko config: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return "", fmt.Errorf("parse generated apko config: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("%w: generated apko config root", ErrIntegerBatchPlan)
	}
	annotations, err := integerYAMLMapping(document.Content[0], "annotations")
	if err != nil {
		return "", err
	}
	values := map[string]string{
		"org.opencontainers.image.title":       options.Title,
		"org.opencontainers.image.description": options.Description,
		"org.opencontainers.image.version":     options.Version,
		"org.opencontainers.image.revision":    options.SourceSHA,
		"org.opencontainers.image.created":     options.CreatedAt.UTC().Format(time.RFC3339),
		"org.verity.publication.id":            options.PublicationID,
	}
	for key, value := range values {
		setIntegerYAMLScalar(annotations, key, value)
	}
	encoded, err := yaml.Marshal(&document)
	if err != nil {
		return "", fmt.Errorf("marshal publish apko config: %w", err)
	}
	file, err := os.CreateTemp("", "integer-publish-*.apko.yaml")
	if err != nil {
		return "", fmt.Errorf("create publish apko config: %w", err)
	}
	path = file.Name()
	if closeErr := file.Close(); closeErr != nil {
		return "", errors.Join(fmt.Errorf("close publish apko config: %w", closeErr), os.Remove(path))
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", errors.Join(fmt.Errorf("write publish apko config: %w", err), os.Remove(path))
	}
	return path, nil
}

func integerYAMLMapping(root *yaml.Node, key string) (*yaml.Node, error) {
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value != key {
			continue
		}
		value := root.Content[index+1]
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%w: apko %s must be a mapping", ErrIntegerBatchPlan, key)
		}
		return value, nil
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root.Content = append(root.Content, keyNode, valueNode)
	return valueNode, nil
}

func setIntegerYAMLScalar(mapping *yaml.Node, key, value string) {
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	mapping.Content = append(
		mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

type execIntegerImageRunner struct{}

func (execIntegerImageRunner) Run(ctx context.Context, command IntegerImageCommand) ([]byte, error) {
	process := exec.CommandContext(ctx, command.Name, command.Args...)
	process.Dir = command.Dir
	process.Stderr = os.Stderr
	if command.CaptureOutput {
		return process.Output()
	}
	process.Stdout = os.Stdout
	if err := process.Run(); err != nil {
		return nil, fmt.Errorf("run %s: %w", command.ID(), err)
	}
	return nil, nil
}
