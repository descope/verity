// Package catalog generates the catalog.json published to the reports branch
// and consumed by the verity website.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/verity-org/verity/internal/integer/apkindex"
	"github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/discovery"
	"github.com/verity-org/verity/internal/integer/eol"
)

// Catalog is the top-level structure consumed by the verity website.
type Catalog struct {
	GeneratedAt string  `json:"generatedAt"`
	Registry    string  `json:"registry"`
	Images      []Image `json:"images"`
}

// Image represents a single named image with all its version streams.
type Image struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Versions    []Version `json:"versions"`
}

// Version represents one version stream (e.g. "1.26" for Go 1.26).
type Version struct {
	Version  string    `json:"version"`
	Latest   bool      `json:"latest,omitempty"`
	EOL      string    `json:"eol,omitempty"`
	Variants []Variant `json:"variants"`
}

// Variant represents one built type (default, dev, fips) within a version.
type Variant struct {
	Type    string   `json:"type"`
	Tags    []string `json:"tags"`
	Ref     string   `json:"ref"`    // primary published ref (registry/name:tag)
	Digest  string   `json:"digest"` // empty if build report unavailable
	BuiltAt string   `json:"builtAt"`
	Status  string   `json:"status"` // "success" | "failure" | "unknown"
}

// buildReport matches the JSON written by .github/scripts/push-reports.sh.
type buildReport struct {
	Digest  string `json:"digest"`
	Status  string `json:"status"`
	BuiltAt string `json:"built_at"`
}

// Generate walks imagesDir, resolves versions from pkgs (APKINDEX packages),
// merges build reports from reportsDir, enriches with EOL data, and returns a Catalog.
//
// reportsDir may be empty (all variants get status "unknown").
// A non-empty reportsDir that does not exist is an error.
// eolFetcher may be nil (EOL data falls back to YAML definitions).
func Generate(imagesDir, reportsDir, registry string, pkgs []apkindex.Package, eolFetcher eol.Fetcher) (*Catalog, error) {
	if reportsDir != "" {
		if _, err := os.Stat(reportsDir); err != nil {
			return nil, fmt.Errorf("reports dir %q: %w", reportsDir, err)
		}
	}

	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, fmt.Errorf("reading images dir %q: %w", imagesDir, err)
	}

	var images []Image

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		defPath := filepath.Join(imagesDir, entry.Name())
		def, err := config.LoadImage(defPath)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", defPath, err)
		}

		img, err := buildImage(def, registry, reportsDir, pkgs, eolFetcher)
		if err != nil {
			return nil, fmt.Errorf("building catalog entry for %q: %w", def.Name, err)
		}
		images = append(images, img)
	}

	// Sort images by name for deterministic output.
	sort.Slice(images, func(i, j int) bool {
		return images[i].Name < images[j].Name
	})

	return &Catalog{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Registry:    registry,
		Images:      images,
	}, nil
}

func buildImage(def *config.ImageDef, registry, reportsDir string, pkgs []apkindex.Package, eolFetcher eol.Fetcher) (Image, error) {
	img := Image{
		Name:        def.Name,
		Description: def.Description,
	}

	// Fetch EOL data from endoflife.date API if client is available.
	var eolData eol.EOLData
	if eolFetcher != nil {
		var err error
		eolData, err = eolFetcher.FetchForImage(def.Name)
		if err != nil {
			// Log warning but don't fail — fall back to YAML definitions
			fmt.Fprintf(os.Stderr, "warning: EOL fetch failed for %s: %v\n", def.Name, err)
		}
	}

	// Resolve versions the same way discovery does.
	versions := discovery.ResolveVersions(def, pkgs)

	for _, v := range versions {
		meta := def.Versions[v]

		// Determine EOL: prefer API data, fall back to YAML.
		eolDate := eolData.LookupEOL(v)
		if eolDate == "" {
			eolDate = meta.EOL
		}

		ver := Version{
			Version: v,
			EOL:     eolDate,
		}

		// Build one variant per type, sorted for determinism.
		typeNames := sortedKeys(def.Types)
		for _, typeName := range typeNames {
			tags := []string{v}
			typeTags := discovery.ApplyTypeSuffix(tags, typeName)
			if len(typeTags) == 0 {
				continue
			}
			ref := fmt.Sprintf("%s/%s:%s", registry, def.Name, typeTags[0])
			variant := Variant{
				Type:   typeName,
				Tags:   typeTags,
				Ref:    ref,
				Status: "unknown",
			}
			if reportsDir != "" {
				reportPath := filepath.Join(reportsDir, def.Name, v, typeName, "latest.json")
				if report, err := loadReport(reportPath); err == nil {
					variant.Digest = report.Digest
					variant.BuiltAt = report.BuiltAt
					variant.Status = report.Status
				}
			}
			ver.Variants = append(ver.Variants, variant)
		}

		img.Versions = append(img.Versions, ver)
	}

	latestIdx := findLatestVersion(img.Versions)
	if latestIdx >= 0 {
		img.Versions[latestIdx].Latest = true
		if img.Versions[latestIdx].Version != "latest" {
			for i := range img.Versions[latestIdx].Variants {
				v := &img.Versions[latestIdx].Variants[i]
				latestTag := "latest"
				if v.Type != "default" {
					latestTag = "latest-" + v.Type
				}
				v.Tags = append(v.Tags, latestTag)
			}
		}
	}

	return img, nil
}

func loadReport(path string) (*buildReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r buildReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func sortedKeys(m map[string]config.TypeTemplate) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// findLatestVersion returns the index of the highest non-EOL version.
// If all versions are EOL, the highest version wins.
// Returns -1 if versions is empty.
func findLatestVersion(versions []Version) int {
	if len(versions) == 0 {
		return -1
	}

	bestNonEOL := -1
	bestOverall := 0
	for i, v := range versions {
		if i > 0 && apkindex.VersionLess(versions[bestOverall].Version, v.Version) {
			bestOverall = i
		}
		if !isEOL(v.EOL) && (bestNonEOL < 0 || apkindex.VersionLess(versions[bestNonEOL].Version, v.Version)) {
			bestNonEOL = i
		}
	}

	if bestNonEOL >= 0 {
		return bestNonEOL
	}
	return bestOverall
}

func isEOL(eolDate string) bool {
	if eolDate == "" {
		return false
	}
	t, err := time.Parse("2006-01-02", eolDate)
	if err != nil {
		return false
	}
	return time.Now().After(t)
}
