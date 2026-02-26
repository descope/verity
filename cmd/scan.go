package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// CopaConfig represents the copa-config.yaml structure.
type CopaConfig struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Target     TargetSpec  `yaml:"target,omitempty"`
	Images     []ImageSpec `yaml:"images"`
}

type ImageSpec struct {
	Name      string      `yaml:"name"`
	Image     string      `yaml:"image"`
	Tags      TagStrategy `yaml:"tags"`
	Target    TargetSpec  `yaml:"target,omitempty"`
	Platforms []string    `yaml:"platforms,omitempty"`
}

type TargetSpec struct {
	Registry string `yaml:"registry,omitempty"`
	Tag      string `yaml:"tag,omitempty"`
}

type TagStrategy struct {
	Strategy string   `yaml:"strategy"`
	Pattern  string   `yaml:"pattern,omitempty"`
	MaxTags  int      `yaml:"maxTags,omitempty"`
	List     []string `yaml:"list,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty"`
}

// ScanCommand generates Trivy vulnerability reports for all images in copa-config.yaml.
var ScanCommand = &cli.Command{
	Name:  "scan",
	Usage: "Scan all images from copa-config.yaml and generate Trivy reports in parallel",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "config",
			Aliases:  []string{"c"},
			Usage:    "Path to copa-config.yaml",
			Required: true,
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Output directory for Trivy reports",
			Value:   "reports",
		},
		&cli.IntFlag{
			Name:  "parallel",
			Usage: "Number of parallel scans",
			Value: 5,
		},
		&cli.StringFlag{
			Name:  "target-registry",
			Usage: "Target registry to check for existing patched images (e.g., ghcr.io/verity-org)",
		},
		&cli.StringFlag{
			Name:  "trivy-server",
			Usage: "Trivy server address (e.g., http://localhost:4954) for parallel scanning. If not set, uses trivy image directly.",
		},
		&cli.BoolFlag{
			Name:  "patched-only",
			Usage: "Scan only patched images in the target registry (skip source images). Requires --target-registry.",
		},
	},
	Action: func(c *cli.Context) error {
		configPath := c.String("config")
		outputDir := c.String("output")
		parallel := c.Int("parallel")
		targetRegistry := c.String("target-registry")
		trivyServer := c.String("trivy-server")
		patchedOnly := c.Bool("patched-only")

		if patchedOnly && targetRegistry == "" {
			return errPatchedOnlyNeedsTarget
		}

		// Read and parse copa-config.yaml
		yamlFile, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		var config CopaConfig
		if err := yaml.Unmarshal(yamlFile, &config); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}

		// Create output directory
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		// Discover all image:tag combinations
		type scanJob struct {
			name       string
			imageRef   string
			outputFile string
			isPatched  bool
		}

		var jobs []scanJob
		for _, imageSpec := range config.Images {
			tags, err := findTagsToPatch(&imageSpec)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to discover tags for '%s': %v\n", imageSpec.Name, err)
				continue
			}

			for _, tag := range tags {
				// Source image scan (skipped in patched-only mode)
				if !patchedOnly {
					sourceRef := fmt.Sprintf("%s:%s", imageSpec.Image, tag)
					sourceFile := filepath.Join(outputDir, sanitizeFilename(sourceRef)+".json")
					jobs = append(jobs, scanJob{
						name:       imageSpec.Name,
						imageRef:   sourceRef,
						outputFile: sourceFile,
						isPatched:  false,
					})
				}

				// Patched image scan (when target registry is specified)
				if targetRegistry != "" {
					patchedRef := fmt.Sprintf("%s/%s:%s-patched", targetRegistry, imageSpec.Name, tag)
					patchedFile := filepath.Join(outputDir, sanitizeFilename(patchedRef)+".json")
					jobs = append(jobs, scanJob{
						name:       imageSpec.Name,
						imageRef:   patchedRef,
						outputFile: patchedFile,
						isPatched:  true,
					})
				}
			}
		}

		fmt.Fprintf(os.Stderr, "Scanning %d images in parallel (concurrency: %d)...\n", len(jobs), parallel)

		// Scan images in parallel
		var wg sync.WaitGroup
		semaphore := make(chan struct{}, parallel)
		errChan := make(chan error, len(jobs))

		for _, job := range jobs {
			wg.Add(1)
			go func(j scanJob) {
				defer wg.Done()
				semaphore <- struct{}{}        // Acquire
				defer func() { <-semaphore }() // Release

				if err := scanImage(j.imageRef, j.outputFile, j.isPatched, trivyServer); err != nil {
					errChan <- fmt.Errorf("%s: %w", j.imageRef, err)
				} else {
					fmt.Fprintf(os.Stderr, "✓ %s\n", j.imageRef)
				}
			}(job)
		}

		wg.Wait()
		close(errChan)

		// Collect errors
		var scanErrors []error
		for err := range errChan {
			scanErrors = append(scanErrors, err)
		}

		if len(scanErrors) > 0 {
			fmt.Fprintf(os.Stderr, "\nWarnings (%d scans failed):\n", len(scanErrors))
			for _, err := range scanErrors {
				fmt.Fprintf(os.Stderr, "  - %v\n", err)
			}
		}

		successCount := len(jobs) - len(scanErrors)
		fmt.Fprintf(os.Stderr, "\nScan complete: %d/%d successful\n", successCount, len(jobs))
		fmt.Fprintf(os.Stderr, "Reports saved to: %s\n", outputDir)

		return nil
	},
}

func scanImage(imageRef, outputFile string, isPatched bool, trivyServer string) error {
	ctx := context.Background()
	var cmd *exec.Cmd

	if trivyServer != "" {
		// Use Trivy server mode (client pulls image, uses server's DB)
		cmd = exec.CommandContext(ctx, "trivy", "image",
			"--server", trivyServer,
			"--vuln-type", "os,library",
			"--format", "json",
			"--quiet",
			imageRef,
		)
	} else {
		// Use Trivy standalone mode (direct DB access)
		cmd = exec.CommandContext(ctx, "trivy", "image",
			"--vuln-type", "os,library",
			"--format", "json",
			"--quiet",
			imageRef,
		)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if isPatched {
			// Patched image might not exist yet, create empty report
			emptyReport := map[string]any{
				"ArtifactName": imageRef,
				"Results":      []any{},
			}
			data, marshalErr := json.MarshalIndent(emptyReport, "", "  ")
			if marshalErr != nil {
				return fmt.Errorf("failed to marshal empty report: %w", marshalErr)
			}
			return os.WriteFile(outputFile, data, 0o644)
		}
		return fmt.Errorf("trivy scan failed: %w\nOutput: %s", err, string(output))
	}

	return os.WriteFile(outputFile, output, 0o644)
}

func sanitizeFilename(filename string) string {
	// Replace unsafe characters for filenames
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, ":", "_")
	return filename
}

var (
	errUnknownStrategy        = errors.New("unknown tag strategy")
	errPatchedOnlyNeedsTarget = errors.New("--patched-only requires --target-registry to be set")
)

// findTagsToPatch discovers tags for an image (reused from discover logic).
func findTagsToPatch(spec *ImageSpec) ([]string, error) {
	repo, err := name.NewRepository(spec.Image)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository: %w", err)
	}

	switch spec.Tags.Strategy {
	case "list":
		return spec.Tags.List, nil
	case "pattern":
		return findTagsByPattern(repo, spec)
	case "latest":
		return findTagsByLatest(repo, spec)
	default:
		return nil, fmt.Errorf("%w: %s", errUnknownStrategy, spec.Tags.Strategy)
	}
}

func findTagsByLatest(repo name.Repository, spec *ImageSpec) ([]string, error) {
	allTags, err := remote.List(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, err
	}

	filteredTags := excludeTags(allTags, spec.Tags.Exclude)
	versions := []*semver.Version{}
	for _, t := range filteredTags {
		// Allow prerelease versions (e.g., "1.2.3-ubuntu") since pattern matching already filters
		if v, err := semver.NewVersion(t); err == nil {
			versions = append(versions, v)
		}
	}

	if len(versions) == 0 {
		return []string{}, nil
	}

	sort.Sort(semver.Collection(versions))
	return []string{versions[len(versions)-1].Original()}, nil
}

func findTagsByPattern(repo name.Repository, spec *ImageSpec) ([]string, error) {
	allTags, err := remote.List(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, err
	}

	pattern, err := regexp.Compile(spec.Tags.Pattern)
	if err != nil {
		return nil, err
	}

	matchingTags := []string{}
	for _, tag := range allTags {
		if pattern.MatchString(tag) {
			matchingTags = append(matchingTags, tag)
		}
	}

	matchingTags = excludeTags(matchingTags, spec.Tags.Exclude)
	versions := []*semver.Version{}
	for _, t := range matchingTags {
		// Allow prerelease versions (e.g., "1.2.3-ubuntu") since pattern matching already filters
		if v, err := semver.NewVersion(t); err == nil {
			versions = append(versions, v)
		}
	}

	if len(versions) == 0 {
		return []string{}, nil
	}

	sort.Sort(semver.Collection(versions))

	if spec.Tags.MaxTags > 0 && len(versions) > spec.Tags.MaxTags {
		versions = versions[len(versions)-spec.Tags.MaxTags:]
	}

	result := make([]string, len(versions))
	for i, v := range versions {
		result[i] = v.Original()
	}
	return result, nil
}

func excludeTags(tags, exclusions []string) []string {
	if len(exclusions) == 0 {
		return tags
	}
	exclusionSet := make(map[string]struct{})
	for _, ex := range exclusions {
		exclusionSet[ex] = struct{}{}
	}
	result := []string{}
	for _, tag := range tags {
		if _, found := exclusionSet[tag]; !found {
			result = append(result, tag)
		}
	}
	return result
}
