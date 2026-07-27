package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/verity-org/verity/internal/discovery"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

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
	for _, result := range results {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "  BUILD: target resolution failed: %v\n", result.err)
		}
		if result.dirty {
			dirty = append(dirty, keep(items[result.idx]))
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
	command := exec.CommandContext(ctx, trivy, "image", "--vuln-type", "os,library", "--format", "json", "--quiet", ref)
	out, err := command.CombinedOutput()
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
	for _, result := range report.Results {
		count += len(result.Vulnerabilities)
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
	images, err := intconfig.LoadImageDefinitions(imagesDir)
	if err != nil {
		return nil, fmt.Errorf("reading integer image names: %w", err)
	}
	names := make(map[string]struct{}, len(images))
	for _, image := range images {
		names[image.Definition.Name] = struct{}{}
	}
	return names, nil
}
