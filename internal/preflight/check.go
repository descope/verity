package preflight

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/crane"

	"github.com/verity-org/verity/internal/discovery"
)

// CheckResult captures the preflight decision for a single image.
type CheckResult struct {
	NeedsWork bool
	Reason    string
}

// digestFn resolves the current upstream digest for a reference. It is a
// package-level variable so tests can swap in a fake without network calls.
var digestFn = func(ref string) (string, error) {
	return crane.Digest(ref)
}

// extractTag returns the tag portion of a full image reference,
// e.g. "mirror.gcr.io/library/nginx:1.29.3" yields "1.29.3".
// Digest-pinned references (containing "@") return "" to signal
// that the image should always be rebuilt.
func extractTag(source string) string {
	if strings.Contains(source, "@") {
		return ""
	}
	if i := strings.LastIndex(source, ":"); i >= 0 {
		return source[i+1:]
	}
	return "latest"
}

// checkCopaImage evaluates whether a single Copa image needs work.
//
// Gate 1: If the image is not in the manifest → first time → needs work.
// Gate 2: Fetch current upstream digest. If different from stored → needs work.
// Gate 3: If digest unchanged but patched image still has any reported vulns → needs work.
// Otherwise: skip.
//
// PatchedVulns is populated from `trivy image` output without
// --ignore-unfixed, so it counts every reported vuln, fixable or not.
func checkCopaImage(img *discovery.DiscoveredImage, manifest Manifest) CheckResult {
	tag := extractTag(img.Source)
	if tag == "" {
		return CheckResult{NeedsWork: true, Reason: img.Name + ": digest-pinned ref (always build)"}
	}
	key := ManifestKey(img.Name, tag)

	entry, exists := manifest[key]
	if !exists {
		return CheckResult{NeedsWork: true, Reason: key + ": first time (not in manifest)"}
	}

	currentDigest, err := digestFn(img.Source)
	if err != nil {
		// If we can't check, err on the side of building.
		return CheckResult{NeedsWork: true, Reason: fmt.Sprintf("%s: digest lookup failed: %v", key, err)}
	}

	if currentDigest != entry.UpstreamDigest {
		return CheckResult{NeedsWork: true, Reason: key + ": upstream digest changed"}
	}

	if entry.PatchedVulns > 0 {
		noun, verb := "vulns", "remain"
		if entry.PatchedVulns == 1 {
			noun, verb = "vuln", "remains"
		}
		return CheckResult{NeedsWork: true, Reason: fmt.Sprintf("%s: %d %s %s in patched image", key, entry.PatchedVulns, noun, verb)}
	}

	return CheckResult{NeedsWork: false, Reason: key + ": unchanged, 0 vulns — skipping"}
}

// filterCopaImagesWithManifest returns only images that need work according to
// the preflight manifest. This is the internal implementation used by both
// FilterCopaImages and tests.
func filterCopaImagesWithManifest(images []discovery.DiscoveredImage, manifest Manifest) ([]discovery.DiscoveredImage, error) {
	type indexedResult struct {
		idx   int
		check CheckResult
	}

	results := make([]indexedResult, len(images))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for i, img := range images {
		wg.Add(1)
		go func(idx int, img discovery.DiscoveredImage) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = indexedResult{idx: idx, check: checkCopaImage(&img, manifest)}
		}(i, img)
	}
	wg.Wait()

	var needed []discovery.DiscoveredImage
	for i, r := range results {
		if r.check.NeedsWork {
			fmt.Fprintf(os.Stderr, "  BUILD: %s\n", r.check.Reason)
			needed = append(needed, images[i])
		} else {
			fmt.Fprintf(os.Stderr, "  SKIP:  %s\n", r.check.Reason)
		}
	}

	return needed, nil
}

// FilterCopaImages fetches the preflight manifest and filters images.
func FilterCopaImages(images []discovery.DiscoveredImage, repo, branch, token string) ([]discovery.DiscoveredImage, error) {
	manifest, err := FetchManifest(repo, branch, token)
	if err != nil {
		return nil, fmt.Errorf("fetching preflight manifest: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Preflight: loaded manifest with %d entries\n", len(manifest))
	return filterCopaImagesWithManifest(images, manifest)
}
