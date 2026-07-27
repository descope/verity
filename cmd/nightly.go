package cmd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/discovery"
	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

const (
	nightlyFamilyCopa    = "copa"
	nightlyFamilyInteger = integerCommandName
)

var (
	errUnsupportedNightlyFamily = errors.New("unsupported nightly family")
	errMissingGitHubToken       = errors.New("GH_TOKEN or GITHUB_TOKEN is required for workflow dispatch")
	errSourceTagUnavailable     = errors.New("source ref is digest-pinned or tagless")
	errMissingTargetRegistry    = errors.New("target registry missing")
	errMissingIntegerTags       = errors.New("integer image has no tags")
	errNoNonEmptyIntegerTags    = errors.New("integer image has no non-empty tags")
	errMissingIntegerRegistry   = errors.New("registry missing for integer image")
	errGitHubDispatchStatus     = errors.New("github dispatch returned non-success status")

	craneDigest        = crane.Digest
	dispatchRetrySleep = time.Sleep
	githubAPIBaseURL   = "https://api.github.com"
	githubHTTPClient   = http.DefaultClient
	trivyVulnCountFor  = nightlyTrivyVulnCount
)

func nightlyPlanCopa(ctx context.Context, cmd *cli.Command, force bool, parallel int) ([]discovery.DiscoveredImage, error) {
	cfg, err := discovery.LoadConfig(cmd.String("config"))
	if err != nil {
		return nil, fmt.Errorf("loading copa config: %w", err)
	}
	charts, err := discovery.LoadChartsFile(cmd.String("charts-file"))
	if err != nil {
		return nil, fmt.Errorf("loading charts file: %w", err)
	}
	cfg.Charts = append(cfg.Charts, charts...)
	vc, err := discovery.LoadVerityConfig(cmd.String("verity-config"))
	if err != nil {
		return nil, fmt.Errorf("loading verity config: %w", err)
	}

	overrides := maps.Clone(cfg.Overrides)
	if overrides == nil {
		overrides = maps.Clone(vc.Overrides)
	} else {
		maps.Copy(overrides, vc.Overrides)
	}
	unpatchable := make(map[string]struct{}, len(vc.UnpatchableImages))
	for _, n := range vc.UnpatchableImages {
		if n = strings.TrimSpace(n); n != "" {
			unpatchable[n] = struct{}{}
		}
	}
	excludeNames, err := integerImageNames(cmd.String("images-dir"))
	if err != nil {
		return nil, err
	}
	images, err := discovery.DiscoverWithChartValues(cfg, cmd.String("target-registry"), overrides, vc.ChartValues, excludeNames, unpatchable)
	if err != nil {
		return nil, fmt.Errorf("discovering copa images: %w", err)
	}
	images = filterCopaImagesByName(images, cmd.String("only"))

	return filterDirty(ctx, images, parallel, func(img discovery.DiscoveredImage) ([]nightlyScanTarget, string, error) {
		targetRef, err := copaTargetRef(&img)
		if err != nil {
			return nil, "", err
		}
		return []nightlyScanTarget{{ref: targetRef, label: targetRef}}, img.Name + " " + targetRef, nil
	}, force, func(i discovery.DiscoveredImage) discovery.DiscoveredImage { return i })
}

func nightlyPlanInteger(ctx context.Context, cmd *cli.Command, force bool, parallel int) ([]intdiscovery.DiscoveredImage, error) {
	cfg, err := intconfig.LoadConfig(cmd.String("integer-config"))
	if err != nil {
		return nil, fmt.Errorf("loading integer config: %w", err)
	}
	registry := cmd.String("target-registry")
	if registry == "" {
		registry = cfg.Target.Registry
	}

	var pkgs []apkindex.Package
	if apkIndexURL := cmd.String("apkindex-url"); apkIndexURL != "" {
		pkgs, err = apkindex.Fetch(apkIndexURL, cmd.String("cache-dir"), apkindex.DefaultCacheMaxAge)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: APKINDEX unavailable (%v) — using versions map only\n", err)
		}
	}
	imagesDir, err := filepath.Abs(cmd.String("images-dir"))
	if err != nil {
		return nil, fmt.Errorf("resolving images dir: %w", err)
	}
	images, err := intdiscovery.DiscoverFromFiles(intdiscovery.Options{
		ImagesDir: imagesDir,
		Registry:  registry,
		Packages:  pkgs,
		GenDir:    cmd.String("gen-dir"),
	})
	if err != nil {
		return nil, fmt.Errorf("discovering integer images: %w", err)
	}
	images = filterIntegerImagesByName(images, cmd.String("only"))

	return filterDirty(ctx, images, parallel, func(img intdiscovery.DiscoveredImage) ([]nightlyScanTarget, string, error) {
		targetRefs, err := integerTargetRefs(&img)
		if err != nil {
			return nil, "", err
		}
		return targetRefs, img.Name + ":" + img.Version + "-" + img.Type, nil
	}, force, func(i intdiscovery.DiscoveredImage) intdiscovery.DiscoveredImage { return i })
}
