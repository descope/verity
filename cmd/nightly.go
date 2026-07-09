package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	nightlyFamilyInteger = "integer"
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

// NightlyCommand owns scheduled scan/dispatch decisions. It deliberately keeps
// orchestration policy in Go so workflows only install tools and invoke verity.
var NightlyCommand = &cli.Command{
	Name:  "nightly",
	Usage: "Plan and dispatch nightly remediation from current vulnerability scans",
	Commands: []*cli.Command{
		nightlyPlanCmd,
		nightlyDispatchCmd,
	},
}

var nightlyPlanCmd = &cli.Command{
	Name:  "plan",
	Usage: "Scan published Verity images and emit only dirty remediation targets",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "family", Usage: "Image family to plan: copa or integer", Required: true},
		&cli.StringFlag{Name: "config", Usage: "Path to copa-config.yaml", Value: "copa-config.yaml"},
		&cli.StringFlag{Name: "charts-file", Usage: "Path to Chart.yaml", Value: "Chart.yaml"},
		&cli.StringFlag{Name: "verity-config", Usage: "Path to verity.yaml", Value: "verity.yaml"},
		&cli.StringFlag{Name: "integer-config", Usage: "Path to integer.yaml", Value: "integer.yaml"},
		&cli.StringFlag{Name: "images-dir", Usage: "Path to images/", Value: "images"},
		&cli.StringFlag{Name: "target-registry", Usage: "Target registry override"},
		&cli.StringFlag{Name: "apkindex-url", Usage: "Wolfi APKINDEX URL", Value: apkindex.DefaultAPKINDEXURL},
		&cli.StringFlag{Name: "cache-dir", Usage: "APKINDEX cache dir"},
		&cli.StringFlag{Name: "gen-dir", Usage: "Generated apko config directory"},
		&cli.StringFlag{Name: "only", Usage: "Comma-separated image names to consider"},
		&cli.IntFlag{Name: "parallel", Usage: "Number of concurrent target scans", Value: 6},
		&cli.BoolFlag{Name: "force", Usage: "Bypass scan skip and emit every considered target"},
		&cli.StringFlag{Name: "output", Usage: "Write dispatch matrix JSON to this file"},
		&cli.StringFlag{Name: "github-output", Usage: "Append count/images outputs to this GitHub output file"},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		family := cmd.String("family")
		force := cmd.Bool("force")
		parallel := max(cmd.Int("parallel"), 1)

		var data []byte
		var count int
		var err error
		switch family {
		case nightlyFamilyCopa:
			var items []discovery.DiscoveredImage
			items, err = nightlyPlanCopa(ctx, cmd, force, parallel)
			if err != nil {
				return err
			}
			if items == nil {
				items = []discovery.DiscoveredImage{}
			}
			count = len(items)
			data, err = json.Marshal(items)
		case nightlyFamilyInteger:
			var items []intdiscovery.DiscoveredImage
			items, err = nightlyPlanInteger(ctx, cmd, force, parallel)
			if err != nil {
				return err
			}
			if items == nil {
				items = []intdiscovery.DiscoveredImage{}
			}
			count = len(items)
			data, err = json.Marshal(items)
		default:
			return fmt.Errorf("%w: %q; want %q or %q", errUnsupportedNightlyFamily, family, nightlyFamilyCopa, nightlyFamilyInteger)
		}
		if err != nil {
			return err
		}

		if out := cmd.String("output"); out != "" {
			if err := os.WriteFile(out, data, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", out, err)
			}
		}
		if ghOut := cmd.String("github-output"); ghOut != "" {
			if err := appendGitHubMatrixOutput(ghOut, count, data); err != nil {
				return err
			}
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	},
}

var nightlyDispatchCmd = &cli.Command{
	Name:  "dispatch",
	Usage: "Dispatch a nightly remediation matrix through the GitHub Actions API",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "family", Usage: "Image family to dispatch: copa or integer", Required: true},
		&cli.StringFlag{Name: "input", Usage: "Dispatch matrix JSON file", Required: true},
		&cli.StringFlag{Name: "repo", Usage: "GitHub repository owner/name", Required: true},
		&cli.StringFlag{Name: "ref", Usage: "Git ref for workflow dispatch", Required: true},
		&cli.StringFlag{Name: "workflow", Usage: "Workflow file name; defaults from --family"},
		&cli.IntFlag{Name: "retries", Usage: "Dispatch retries per item", Value: 5},
		&cli.DurationFlag{Name: "throttle", Usage: "Delay between successful dispatches", Value: 2 * time.Second},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		workflow := cmd.String("workflow")
		if workflow == "" {
			switch cmd.String("family") {
			case nightlyFamilyCopa:
				workflow = "patch-image.yaml"
			case nightlyFamilyInteger:
				workflow = "integer-build-image.yaml"
			default:
				return fmt.Errorf("%w: %q", errUnsupportedNightlyFamily, cmd.String("family"))
			}
		}
		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}
		if token == "" {
			return errMissingGitHubToken
		}

		inputs, err := nightlyDispatchInputs(cmd.String("family"), cmd.String("input"))
		if err != nil {
			return err
		}
		for i, in := range inputs {
			if err := dispatchWorkflow(ctx, token, cmd.String("repo"), workflow, cmd.String("ref"), in, cmd.Int("retries")); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Dispatched %d/%d to %s\n", i+1, len(inputs), workflow)
			if i+1 < len(inputs) && cmd.Duration("throttle") > 0 {
				time.Sleep(cmd.Duration("throttle"))
			}
		}
		fmt.Fprintf(os.Stderr, "✓ Dispatched %d %s remediation run(s)\n", len(inputs), cmd.String("family"))
		return nil
	},
}

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

type nightlyScanTarget struct {
	ref   string
	label string
}

func filterDirty[T any](ctx context.Context, items []T, parallel int, target func(T) ([]nightlyScanTarget, string, error), force bool, keep func(T) T) ([]T, error) {
	if force {
		fmt.Fprintf(os.Stderr, "Force mode: emitting %d target(s) without scan skip\n", len(items))
		return items, nil
	}
	type result struct {
		idx   int
		dirty bool
		err   error
	}
	results := make([]result, len(items))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(idx int, item T) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			targets, label, err := target(item)
			if err != nil {
				results[idx] = result{idx: idx, dirty: true, err: err}
				return
			}
			decision := scanPublishedTargets(ctx, targets)
			if decision.dirty {
				fmt.Fprintf(os.Stderr, "  BUILD: %s: %s\n", label, decision.reason)
			} else {
				fmt.Fprintf(os.Stderr, "  SKIP:  %s: %s\n", label, decision.reason)
			}
			results[idx] = result{idx: idx, dirty: decision.dirty}
		}(i, item)
	}
	wg.Wait()

	dirty := make([]T, 0)
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "  BUILD: target resolution failed: %v\n", r.err)
		}
		if r.dirty {
			dirty = append(dirty, keep(items[r.idx]))
		}
	}
	fmt.Fprintf(os.Stderr, "Nightly plan: %d/%d target(s) need remediation\n", len(dirty), len(items))
	return dirty, nil
}

type scanDecision struct {
	dirty  bool
	reason string
}

func scanPublishedTargets(ctx context.Context, targets []nightlyScanTarget) scanDecision {
	if len(targets) == 0 {
		return scanDecision{dirty: true, reason: "no target tags to scan"}
	}
	seenDigests := map[string]struct{}{}
	for _, target := range targets {
		if target.label == "" {
			target.label = target.ref
		}
		digest, err := craneDigest(target.ref)
		if err != nil {
			return scanDecision{dirty: true, reason: target.label + " missing or digest lookup failed: " + err.Error()}
		}
		if _, ok := seenDigests[digest]; ok {
			continue
		}
		seenDigests[digest] = struct{}{}

		count, err := trivyVulnCountFor(ctx, target.ref)
		if err != nil {
			return scanDecision{dirty: true, reason: target.label + " scan failed: " + err.Error()}
		}
		if count > 0 {
			return scanDecision{dirty: true, reason: fmt.Sprintf("%s has %d vulnerabilities", target.label, count)}
		}
	}
	return scanDecision{dirty: false, reason: fmt.Sprintf("%d published tag(s) are clean", len(targets))}
}

func nightlyTrivyVulnCount(ctx context.Context, ref string) (int, error) {
	trivy, err := exec.LookPath("trivy")
	if err != nil {
		return 0, fmt.Errorf("trivy not found in PATH: %w", err)
	}
	c := exec.CommandContext(ctx, trivy, "image", "--vuln-type", "os,library", "--format", "json", "--quiet", ref)
	out, err := c.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	var report struct {
		Results []struct {
			Vulnerabilities []json.RawMessage `json:"Vulnerabilities"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		return 0, fmt.Errorf("parsing trivy report: %w", err)
	}
	count := 0
	for _, r := range report.Results {
		count += len(r.Vulnerabilities)
	}
	return count, nil
}

func copaTargetRef(img *discovery.DiscoveredImage) (string, error) {
	tag := sourceTag(img.Source)
	if tag == "" {
		return "", fmt.Errorf("%w: %q; cannot derive target tag", errSourceTagUnavailable, img.Source)
	}
	if img.TargetRegistry == "" {
		return "", fmt.Errorf("%w for %q", errMissingTargetRegistry, img.Name)
	}
	return strings.TrimRight(img.TargetRegistry, "/") + "/" + img.Name + ":" + tag, nil
}

func integerTargetRef(img *intdiscovery.DiscoveredImage) (string, error) {
	if len(img.Tags) == 0 {
		return "", fmt.Errorf("%w: %s:%s-%s", errMissingIntegerTags, img.Name, img.Version, img.Type)
	}
	if img.Registry == "" {
		return "", fmt.Errorf("%w: %s:%s-%s", errMissingIntegerRegistry, img.Name, img.Version, img.Type)
	}
	return strings.TrimRight(img.Registry, "/") + "/" + img.Name + ":" + img.Tags[0], nil
}

func integerTargetRefs(img *intdiscovery.DiscoveredImage) ([]nightlyScanTarget, error) {
	if len(img.Tags) == 0 {
		return nil, fmt.Errorf("%w: %s:%s-%s", errMissingIntegerTags, img.Name, img.Version, img.Type)
	}
	if img.Registry == "" {
		return nil, fmt.Errorf("%w: %s:%s-%s", errMissingIntegerRegistry, img.Name, img.Version, img.Type)
	}
	base := strings.TrimRight(img.Registry, "/") + "/" + img.Name
	targets := make([]nightlyScanTarget, 0, len(img.Tags))
	for _, tag := range img.Tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			ref := base + ":" + tag
			targets = append(targets, nightlyScanTarget{ref: ref, label: ref})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: %s:%s-%s", errNoNonEmptyIntegerTags, img.Name, img.Version, img.Type)
	}
	return targets, nil
}

func sourceTag(ref string) string {
	if strings.Contains(ref, "@") {
		return ""
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon <= lastSlash {
		return "latest"
	}
	return ref[lastColon+1:]
}

func integerImageNames(imagesDir string) (map[string]struct{}, error) {
	files, err := intconfig.ImageFilePaths(imagesDir)
	if err != nil {
		return nil, fmt.Errorf("reading integer image names: %w", err)
	}
	names := make(map[string]struct{}, len(files))
	for _, f := range files {
		rel, err := filepath.Rel(imagesDir, f)
		if err != nil {
			return nil, fmt.Errorf("relativizing integer image path %s: %w", f, err)
		}
		names[strings.TrimSuffix(filepath.ToSlash(rel), yamlExt)] = struct{}{}
	}
	return names, nil
}

func appendGitHubMatrixOutput(path string, count int, data []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening GitHub output %s: %w", path, err)
	}
	return appendGitHubMatrixOutputTo(f, path, count, data)
}

func appendGitHubMatrixOutputTo(w io.WriteCloser, path string, count int, data []byte) (retErr error) {
	defer func() {
		if cerr := w.Close(); cerr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing GitHub output %s: %w", path, cerr))
		}
	}()
	if _, err := fmt.Fprintf(w, "count=%d\nimages<<__VERITY_NIGHTLY_JSON__\n%s\n__VERITY_NIGHTLY_JSON__\n", count, data); err != nil {
		return fmt.Errorf("writing GitHub output %s: %w", path, err)
	}
	return nil
}

func nightlyDispatchInputs(family, inputPath string) ([]map[string]string, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("reading dispatch matrix %s: %w", inputPath, err)
	}
	switch family {
	case nightlyFamilyCopa:
		var items []discovery.DiscoveredImage
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("parsing copa matrix: %w", err)
		}
		out := make([]map[string]string, 0, len(items))
		for _, item := range items {
			in := map[string]string{
				"image-name":      item.Name,
				"source-ref":      item.Source,
				"target-registry": item.TargetRegistry,
				"platforms":       item.Platforms,
			}
			if item.GoVcsURL != "" {
				in["go-vcs-url"] = item.GoVcsURL
			}
			out = append(out, in)
		}
		return out, nil
	case nightlyFamilyInteger:
		var items []intdiscovery.DiscoveredImage
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("parsing integer matrix: %w", err)
		}
		out := make([]map[string]string, 0, len(items))
		for _, item := range items {
			out = append(out, map[string]string{
				"image":    item.Name,
				"version":  item.Version,
				"type":     item.Type,
				"tags":     strings.Join(item.Tags, ","),
				"registry": item.Registry,
			})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%w: %q", errUnsupportedNightlyFamily, family)
	}
}

func dispatchWorkflow(ctx context.Context, token, repo, workflow, ref string, inputs map[string]string, retries int) error {
	if retries < 1 {
		retries = 1
	}
	body, err := json.Marshal(map[string]any{
		"ref":    ref,
		"inputs": inputs,
	})
	if err != nil {
		return fmt.Errorf("marshalling dispatch body: %w", err)
	}
	endpoint := fmt.Sprintf("%s/repos/%s/actions/workflows/%s/dispatches", strings.TrimRight(githubAPIBaseURL, "/"), repo, neturl.PathEscape(workflow))
	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating dispatch request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("Content-Type", "application/json")

		resp, err := githubHTTPClient.Do(req)
		switch {
		case err != nil:
			lastErr = err
		case resp.StatusCode == http.StatusNoContent:
			if resp.Body != nil {
				if cerr := resp.Body.Close(); cerr != nil {
					return fmt.Errorf("closing github dispatch response: %w", cerr)
				}
			}
			return nil
		default:
			lastErr = githubDispatchResponseError(resp)
		}
		if attempt < retries {
			dispatchRetrySleep(time.Duration(attempt*10) * time.Second)
		}
	}
	return fmt.Errorf("dispatching %s after %d attempt(s): %w", workflow, retries, lastErr)
}

func githubDispatchResponseError(resp *http.Response) error {
	if resp.Body == nil {
		return fmt.Errorf("%w: %s", errGitHubDispatchStatus, resp.Status)
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	closeErr := resp.Body.Close()
	switch {
	case readErr != nil:
		return fmt.Errorf("reading github dispatch error response: %w", readErr)
	case closeErr != nil:
		return fmt.Errorf("closing github dispatch error response: %w", closeErr)
	default:
		return fmt.Errorf("%w: %s: %s", errGitHubDispatchStatus, resp.Status, strings.TrimSpace(string(responseBody)))
	}
}
