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
	"strconv"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal/discovery"
)

var errPatchedOnlyNeedsTarget = errors.New("--patched-only requires --target-registry to be set")

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

		cfg, err := discovery.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		type scanJob struct {
			name       string
			imageRef   string
			outputFile string
			isPatched  bool
		}

		var jobs []scanJob
		for i := range cfg.Images {
			imageSpec := &cfg.Images[i]
			tags, err := discovery.FindTagsToPatch(imageSpec)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to discover tags for '%s': %v\n", imageSpec.Name, err)
				continue
			}

			var existingPatchedTags []string
			if targetRegistry != "" && len(tags) > 0 {
				repo, repoErr := name.NewRepository(fmt.Sprintf("%s/%s", targetRegistry, imageSpec.Name))
				if repoErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to parse target repo for %q: %v; falling back to <tag>-patched\n", imageSpec.Name, repoErr)
				} else {
					listed, listErr := remote.List(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
					if listErr != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to list patched tags for %q: %v; falling back to <tag>-patched\n", imageSpec.Name, listErr)
					} else {
						existingPatchedTags = listed
					}
				}
			}

			for _, tag := range tags {
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

				if targetRegistry != "" {
					patchedTag := latestPatchedTagFromList(existingPatchedTags, tag)
					if patchedTag == "" {
						patchedTag = tag + "-patched"
					}
					patchedRef := fmt.Sprintf("%s/%s:%s", targetRegistry, imageSpec.Name, patchedTag)
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

		var wg sync.WaitGroup
		semaphore := make(chan struct{}, parallel)
		errChan := make(chan error, len(jobs))

		for _, job := range jobs {
			wg.Add(1)
			go func(j scanJob) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				if err := scanImage(j.imageRef, j.outputFile, j.isPatched, trivyServer); err != nil {
					errChan <- fmt.Errorf("%s: %w", j.imageRef, err)
				} else {
					fmt.Fprintf(os.Stderr, "✓ %s\n", j.imageRef)
				}
			}(job)
		}

		wg.Wait()
		close(errChan)

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
		cmd = exec.CommandContext(ctx, "trivy", "image",
			"--server", trivyServer,
			"--vuln-type", "os,library",
			"--format", "json",
			"--quiet",
			imageRef,
		)
	} else {
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

// latestPatchedTagFromList finds the highest-versioned patched tag matching
// "<sourceTag>-patched" or "<sourceTag>-patched-N" from a list of tags.
func latestPatchedTagFromList(tags []string, sourceTag string) string {
	base := regexp.QuoteMeta(sourceTag) + `-patched`
	pattern := regexp.MustCompile(`^` + base + `(-(\d+))?$`)

	bestN := -1
	bestTag := ""
	for _, t := range tags {
		m := pattern.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		n := 0
		if m[2] != "" {
			parsed, err := strconv.Atoi(m[2])
			if err != nil {
				continue
			}
			n = parsed
		}
		if n > bestN {
			bestN = n
			bestTag = t
		}
	}
	return bestTag
}

func sanitizeFilename(filename string) string {
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, ":", "_")
	return filename
}
