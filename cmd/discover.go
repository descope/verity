package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/discovery"
	"github.com/verity-org/verity/internal/preflight"
)

var errMissingGithubRepo = errors.New("--github-repo is required when --preflight is enabled")

// DiscoverCommand enumerates all image+tag combos from three sources:
//   - copa-config.yaml  — standalone images (Copa's domain)
//   - Chart.yaml        — Helm chart dependencies (standard Helm format)
//   - verity.yaml       — tag variant overrides (verity-specific)
var DiscoverCommand = &cli.Command{
	Name:  "discover",
	Usage: "Enumerate all image+tag combos from copa-config.yaml, Chart.yaml, and verity.yaml overrides",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "config",
			Aliases:  []string{"c"},
			Usage:    "Path to copa-config.yaml (standalone images)",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "target-registry",
			Usage: "Override the target registry from config (e.g., ghcr.io/verity-org)",
		},
		&cli.StringFlag{
			Name:  "charts-file",
			Usage: "Helm Chart.yaml whose dependencies: provides chart images",
			Value: "Chart.yaml",
		},
		&cli.StringFlag{
			Name:  "verity-config",
			Usage: "Path to verity.yaml (tag variant overrides)",
			Value: "verity.yaml",
		},
		&cli.StringFlag{
			Name:  "only",
			Usage: "Comma-separated list of image names to include (empty = all)",
		},
		&cli.StringFlag{
			Name:  "exclude-names",
			Usage: "Comma-separated image names to exclude from chart discovery (e.g., Integer/Wolfi image names)",
		},
		&cli.BoolFlag{
			Name:  "preflight",
			Usage: "Enable preflight digest-based skip logic (compares upstream digests with manifest)",
		},
		&cli.StringFlag{
			Name:  "github-repo",
			Usage: "GitHub repository (owner/repo) for preflight manifest lookup",
		},
		&cli.StringFlag{
			Name:  "reports-branch",
			Usage: "Branch where preflight-manifest.json is stored",
			Value: "reports",
		},
	},
	Action: func(_ context.Context, cmd *cli.Command) error {
		cfg, err := discovery.LoadConfig(cmd.String("config"))
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		charts, err := discovery.LoadChartsFile(cmd.String("charts-file"))
		if err != nil {
			return fmt.Errorf("failed to load charts file: %w", err)
		}
		cfg.Charts = append(cfg.Charts, charts...)

		vc, err := discovery.LoadVerityConfig(cmd.String("verity-config"))
		if err != nil {
			return fmt.Errorf("failed to load verity config: %w", err)
		}

		// Merge overrides: verity.yaml takes precedence over copa-config.yaml.
		overrides := maps.Clone(cfg.Overrides)
		if overrides == nil {
			overrides = maps.Clone(vc.Overrides)
		} else {
			maps.Copy(overrides, vc.Overrides)
		}
		// excludeNames covers Wolfi-rebuild dedup (chart-discovered images
		// only — see Discover docstring). UnpatchableImages from verity.yaml
		// is a stronger signal ("this image cannot be patched, regardless
		// of source") and is also merged in here so chart-discovered
		// occurrences emit the "Skipping chart image" log line. Standalone
		// occurrences are filtered by the post-filter below since
		// excludeNames intentionally does not apply to standalone images.
		excludeNames := parseNameSet(cmd.String("exclude-names"))
		unpatchable := make(map[string]struct{}, len(vc.UnpatchableImages))
		for _, n := range vc.UnpatchableImages {
			if n = strings.TrimSpace(n); n != "" {
				unpatchable[n] = struct{}{}
			}
		}
		if len(unpatchable) > 0 {
			if excludeNames == nil {
				excludeNames = make(map[string]struct{})
			}
			for n := range unpatchable {
				excludeNames[n] = struct{}{}
			}
		}

		images, err := discovery.Discover(cfg, cmd.String("target-registry"), overrides, excludeNames)
		if err == nil && len(unpatchable) > 0 {
			images = filterUnpatchable(images, unpatchable)
		}
		if err != nil {
			return fmt.Errorf("failed to discover images: %w", err)
		}

		// --only: filter to specific image names
		if only := cmd.String("only"); only != "" {
			images = filterCopaImagesByName(images, only)
			fmt.Fprintf(os.Stderr, "Filtered to %d images matching --only=%s\n", len(images), only)
		}

		// --preflight: skip images that don't need work
		if cmd.Bool("preflight") {
			repo := cmd.String("github-repo")
			if repo == "" {
				return errMissingGithubRepo
			}
			branch := cmd.String("reports-branch")
			token := os.Getenv("GH_TOKEN")
			if token == "" {
				token = os.Getenv("GITHUB_TOKEN")
			}
			images, err = preflight.FilterCopaImages(images, repo, branch, token)
			if err != nil {
				return fmt.Errorf("preflight filtering failed: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Preflight: %d images need work\n", len(images))
		}

		out, err := json.MarshalIndent(images, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}

		fmt.Fprintln(os.Stdout, string(out))
		return nil
	},
}

// parseNameSet splits a comma-separated string into a set of trimmed,
// non-empty names. Returns nil for empty input.
func parseNameSet(csv string) map[string]struct{} {
	if csv == "" {
		return nil
	}
	set := make(map[string]struct{})
	for n := range strings.SplitSeq(csv, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			set[n] = struct{}{}
		}
	}
	return set
}

// filterUnpatchable drops images whose Name appears in the unpatchable set,
// regardless of source. Necessary because discovery.Discover only consults
// excludeNames for chart-sourced images by design (Wolfi-rebuild dedup
// semantic), and verity.yaml's UnpatchableImages must apply to standalone
// entries too (e.g. cert-manager/cert-manager-openshift-routes lives in
// copa-config.yaml but is distroless and has no Wolfi rebuild yet).
func filterUnpatchable(images []discovery.DiscoveredImage, unpatchable map[string]struct{}) []discovery.DiscoveredImage {
	if len(unpatchable) == 0 {
		return images
	}
	filtered := images[:0]
	for _, img := range images {
		if _, skip := unpatchable[img.Name]; skip {
			fmt.Fprintf(os.Stderr, "Skipping unpatchable image %q from source %q (verity.yaml unpatchableImages)\n", img.Name, img.Source)
			continue
		}
		filtered = append(filtered, img)
	}
	return filtered
}

// filterCopaImagesByName filters images to only those whose Name matches one
// of the comma-separated names.
func filterCopaImagesByName(images []discovery.DiscoveredImage, names string) []discovery.DiscoveredImage {
	allowed := parseNameSet(names)
	if allowed == nil {
		return images
	}

	var filtered []discovery.DiscoveredImage
	for _, img := range images {
		if _, ok := allowed[img.Name]; ok {
			filtered = append(filtered, img)
		}
	}
	return filtered
}
