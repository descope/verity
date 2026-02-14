package internal

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"gopkg.in/yaml.v3"
)

// SiteData is the top-level structure for the catalog JSON consumed by the Astro site.
type SiteData struct {
	GeneratedAt      string      `json:"generatedAt"`
	Registry         string      `json:"registry"`
	Summary          SiteSummary `json:"summary"`
	Charts           []SiteChart `json:"charts"`
	StandaloneImages []SiteImage `json:"standaloneImages"`
}

// SiteSummary aggregates stats across all charts and images.
type SiteSummary struct {
	TotalCharts   int `json:"totalCharts"`
	TotalImages   int `json:"totalImages"`
	TotalVulns    int `json:"totalVulns"`
	FixableVulns  int `json:"fixableVulns"`
}

// SiteChart describes a wrapper Helm chart.
type SiteChart struct {
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	UpstreamVersion string      `json:"upstreamVersion"`
	Description     string      `json:"description"`
	Repository      string      `json:"repository"`
	HelmInstall     string      `json:"helmInstall"`
	Images          []SiteImage `json:"images"`
}

// SiteImage describes a single container image with its vulnerability data.
type SiteImage struct {
	ID              string      `json:"id"`
	OriginalRef     string      `json:"originalRef"`
	PatchedRef      string      `json:"patchedRef"`
	ValuesPath      string      `json:"valuesPath"`
	OS              string      `json:"os"`
	OverriddenFrom  string      `json:"overriddenFrom,omitempty"`
	VulnSummary     VulnSummary `json:"vulnSummary"`
	Vulnerabilities []SiteVuln  `json:"vulnerabilities"`
	ChartName       string      `json:"chartName,omitempty"`
}

// VulnSummary counts vulnerabilities by severity.
type VulnSummary struct {
	Total          int            `json:"total"`
	Fixable        int            `json:"fixable"`
	SeverityCounts map[string]int `json:"severityCounts"`
}

// SiteVuln represents a single vulnerability entry.
type SiteVuln struct {
	ID               string `json:"id"`
	PkgName          string `json:"pkgName"`
	InstalledVersion string `json:"installedVersion"`
	FixedVersion     string `json:"fixedVersion"`
	Severity         string `json:"severity"`
	Title            string `json:"title"`
}

// trivyReportFull is an expanded version of trivyReport that captures severity,
// package info, and OS metadata from Trivy JSON reports.
type trivyReportFull struct {
	Metadata struct {
		OS struct {
			Family string `json:"Family"`
			Name   string `json:"Name"`
		} `json:"OS"`
	} `json:"Metadata"`
	Results []trivyResultFull `json:"Results"`
}

type trivyResultFull struct {
	Vulnerabilities []trivyVulnFull `json:"Vulnerabilities"`
}

type trivyVulnFull struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Severity         string `json:"Severity"`
	Title            string `json:"Title"`
}

// SaveOverrides writes a mapping of sanitized image ref → original tag
// to an overrides.json file in the given directory.
func SaveOverrides(results []*PatchResult, dir string) error {
	overrides := make(map[string]string)
	for _, r := range results {
		if r.OverriddenFrom != "" {
			key := sanitize(r.Original.Reference())
			overrides[key] = r.OverriddenFrom
		}
	}
	if len(overrides) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(overrides, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "overrides.json"), data, 0o644)
}

// SaveImagePaths writes a mapping of sanitized image ref → Helm values path
// so that site data generation can populate valuesPath even for unpatched images.
func SaveImagePaths(results []*PatchResult, dir string) error {
	paths := make(map[string]string)
	for _, r := range results {
		if r.Original.Path != "" {
			key := sanitize(r.Original.Reference())
			paths[key] = r.Original.Path
		}
	}
	if len(paths) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "paths.json"), data, 0o644)
}

// loadImagePaths reads a paths.json file and returns the mapping.
func loadImagePaths(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, "paths.json"))
	if err != nil {
		return nil
	}
	var paths map[string]string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil
	}
	return paths
}

// loadOverrides reads an overrides.json file and returns the mapping.
func loadOverrides(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, "overrides.json"))
	if err != nil {
		return nil
	}
	var overrides map[string]string
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil
	}
	return overrides
}

// GenerateSiteData walks the charts directory and standalone images file
// to produce a catalog.json for the Astro static site.
// Reports are pulled from the OCI registry (embedded in chart packages
// and standalone-reports artifact), not from local files.
func GenerateSiteData(chartsDir, imagesFile, registry, outputPath string) error {
	data := SiteData{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Registry:    registry,
	}

	// Discover wrapper charts (all data pulled from OCI).
	charts, err := discoverCharts(chartsDir, registry)
	if err != nil {
		return fmt.Errorf("discovering charts: %w", err)
	}
	data.Charts = charts

	// Discover standalone images (reports pulled from OCI).
	if imagesFile != "" {
		standalone, err := discoverStandaloneImages(imagesFile, registry)
		if err != nil {
			return fmt.Errorf("discovering standalone images: %w", err)
		}
		data.StandaloneImages = standalone
	}

	// Compute summary
	data.Summary = computeSummary(data.Charts, data.StandaloneImages)

	// Marshal and write
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling site data: %w", err)
	}
	return os.WriteFile(outputPath, out, 0o644)
}

// discoverCharts walks chartsDir/*/Chart.yaml to find wrapper charts.
// All chart data (including reports) is pulled from the OCI registry;
// the local chart directories only provide the chart name.
func discoverCharts(chartsDir, registry string) ([]SiteChart, error) {
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var charts []SiteChart
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		chartYamlPath := filepath.Join(chartsDir, entry.Name(), "Chart.yaml")
		if _, err := os.Stat(chartYamlPath); os.IsNotExist(err) {
			continue
		}

		if registry != "" {
			// Pull ALL versions from OCI (reports are embedded in the chart packages).
			versions, err := discoverRegistryVersions(entry.Name(), "", "", registry)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not discover registry versions for %s: %v\n", entry.Name(), err)
			}
			charts = append(charts, versions...)
		} else {
			// No registry — fall back to local chart parsing (no reports).
			chart, err := parseWrapperChart(chartsDir, entry.Name(), registry)
			if err != nil {
				return nil, fmt.Errorf("parsing chart %s: %w", entry.Name(), err)
			}
			charts = append(charts, chart)
		}
	}

	sort.Slice(charts, func(i, j int) bool {
		if charts[i].Name != charts[j].Name {
			return charts[i].Name < charts[j].Name
		}
		return charts[i].Version > charts[j].Version
	})
	return charts, nil
}

// discoverRegistryVersions queries the GitHub Packages API for all published
// versions of a chart, pulls each one, and returns SiteChart entries with
// full data (including embedded Trivy reports).
// If skipVersion is non-empty, that version is excluded from the results.
func discoverRegistryVersions(chartName, skipVersion, repository, registry string) ([]SiteChart, error) {
	tags, err := listChartTags(registry, chartName)
	if err != nil {
		return nil, err
	}

	var charts []SiteChart
	for _, tag := range tags {
		if skipVersion != "" && tag == skipVersion {
			continue
		}

		tmpDir, err := os.MkdirTemp("", "verity-version-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create temp directory for %s:%s: %v\n", chartName, tag, err)
			continue
		}

		dep := Dependency{
			Name:       chartName,
			Version:    tag,
			Repository: fmt.Sprintf("oci://%s/charts", registry),
		}

		_, dlErr := DownloadChart(dep, tmpDir)
		if dlErr != nil {
			os.RemoveAll(tmpDir)
			fmt.Fprintf(os.Stderr, "Warning: could not pull %s:%s: %v\n", chartName, tag, dlErr)
			continue
		}

		chart, parseErr := parseWrapperChart(tmpDir, chartName, registry)
		os.RemoveAll(tmpDir)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not parse %s:%s: %v\n", chartName, tag, parseErr)
			continue
		}

		charts = append(charts, chart)
	}

	return charts, nil
}

// listChartTags discovers published chart versions. It first tries the
// GitHub Packages API (works with standard GITHUB_TOKEN) and falls back
// to crane.ListTags for non-GitHub registries.
func listChartTags(registry, chartName string) ([]string, error) {
	// Try GitHub Packages API for ghcr.io registries.
	if strings.Contains(registry, "ghcr.io") {
		tags, err := listGitHubPackageTags(registry, chartName)
		if err == nil {
			return tags, nil
		}
		fmt.Fprintf(os.Stderr, "Warning: GitHub API failed for %s, trying crane: %v\n", chartName, err)
	}

	// Fallback to crane for other registries.
	chartRef := fmt.Sprintf("%s/charts/%s", registry, chartName)
	tags, err := crane.ListTags(chartRef)
	if err != nil {
		return nil, fmt.Errorf("listing tags with crane for %s: %w", chartRef, err)
	}
	return tags, nil
}

// listGitHubPackageTags uses the GitHub Packages API to list all tags
// for a chart package. This works with standard GITHUB_TOKEN auth
// (unlike OCI pull which needs packages:read scope for crane).
func listGitHubPackageTags(registry, chartName string) ([]string, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN not set")
	}

	// Extract org from registry: "ghcr.io/descope" → "descope"
	parts := strings.SplitN(registry, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("cannot extract org from registry %q", registry)
	}
	org := parts[1]

	// Package name in API uses %2F for slashes: "charts/prometheus" → "charts%2Fprometheus"
	packageName := "charts%2F" + chartName
	baseURL := fmt.Sprintf("https://api.github.com/orgs/%s/packages/container/%s/versions", org, packageName)

	var tags []string
	perPage := 100
	page := 1

	for {
		url := fmt.Sprintf("%s?per_page=%d&page=%d", baseURL, perPage, page)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
		}

		var pageVersions []struct {
			Metadata struct {
				Container struct {
					Tags []string `json:"tags"`
				} `json:"container"`
			} `json:"metadata"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&pageVersions); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		if len(pageVersions) == 0 {
			break
		}

		for _, v := range pageVersions {
			for _, tag := range v.Metadata.Container.Tags {
				if isVersionTag(tag) {
					tags = append(tags, tag)
				}
			}
		}

		if len(pageVersions) < perPage {
			break
		}
		page++
	}

	return tags, nil
}

// isVersionTag returns true if a tag looks like a chart version
// (e.g. "28.9.1-5") rather than a cosign signature or digest tag.
func isVersionTag(tag string) bool {
	if strings.HasSuffix(tag, ".sig") || strings.HasSuffix(tag, ".att") {
		return false
	}
	if strings.HasPrefix(tag, "sha256-") {
		return false
	}
	return true
}

// parseWrapperChart reads a wrapper chart's metadata, values, and reports.
func parseWrapperChart(chartsDir, name, registry string) (SiteChart, error) {
	chartDir := filepath.Join(chartsDir, name)

	// Parse Chart.yaml
	cf, err := ParseChartFile(filepath.Join(chartDir, "Chart.yaml"))
	if err != nil {
		return SiteChart{}, err
	}

	chart := SiteChart{
		Name:        cf.Name,
		Version:     cf.Version,
		Description: cf.Description,
	}

	// Extract upstream version and repository from dependency
	if len(cf.Dependencies) > 0 {
		dep := cf.Dependencies[0]
		chart.UpstreamVersion = dep.Version
		chart.Repository = dep.Repository
		chart.HelmInstall = fmt.Sprintf("helm install %s oci://%s/charts/%s --version %s",
			cf.Name, registry, cf.Name, cf.Version)
	}

	// Parse values.yaml to find patched image references
	valuesPath := filepath.Join(chartDir, "values.yaml")
	patchedImages, err := parsePatchedValues(valuesPath)
	if err != nil && !os.IsNotExist(err) {
		return SiteChart{}, fmt.Errorf("parsing values: %w", err)
	}

	// Discover reports and match to images
	reportsDir := filepath.Join(chartDir, "reports")
	overrides := loadOverrides(chartDir)
	imagePaths := loadImagePaths(chartDir)
	images, err := matchReportsToImages(reportsDir, patchedImages, overrides, imagePaths, registry, name)
	if err != nil {
		return SiteChart{}, fmt.Errorf("matching reports: %w", err)
	}

	// Filter to only images belonging to this chart version.
	// paths.json records the canonical set of images for the current build;
	// the reports directory may also contain leftover reports from older
	// versions that should not appear on this version's page.
	if len(imagePaths) > 0 {
		filtered := make([]SiteImage, 0, len(imagePaths))
		for _, img := range images {
			if _, ok := imagePaths[img.ID]; ok {
				filtered = append(filtered, img)
			}
		}
		images = filtered
	} else if upstreamDir := filepath.Join(chartDir, "charts", cf.Name); dirExists(upstreamDir) {
		// Fallback for older chart packages that lack paths.json:
		// scan the bundled upstream chart to determine which images
		// belong to this version.
		upstreamImages, scanErr := ScanForImages(upstreamDir)
		if scanErr == nil && len(upstreamImages) > 0 {
			allowed := make(map[string]bool)
			for _, uimg := range upstreamImages {
				allowed[sanitize(uimg.Reference())] = true
			}
			filtered := make([]SiteImage, 0, len(upstreamImages))
			for _, img := range images {
				if allowed[img.ID] {
					filtered = append(filtered, img)
				}
			}
			images = filtered
		}
	}

	chart.Images = images

	return chart, nil
}

// patchedImageInfo holds info extracted from the wrapper chart's values.yaml.
type patchedImageInfo struct {
	Registry   string
	Repository string
	Tag        string
	ValuesPath string
}

// parsePatchedValues reads the wrapper chart's values.yaml and extracts
// patched image references with their values paths.
func parsePatchedValues(valuesPath string) (map[string]patchedImageInfo, error) {
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, err
	}

	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, err
	}

	result := make(map[string]patchedImageInfo)
	collectPatchedImages(values, "", result)
	return result, nil
}

// collectPatchedImages recursively walks a values tree looking for image
// definitions that contain a "-patched" tag.
func collectPatchedImages(node any, path string, result map[string]patchedImageInfo) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}

	// Check if this is an image definition with a patched tag
	if repo, ok := stringVal(m, "repository"); ok {
		if tag, ok := stringVal(m, "tag"); ok && strings.HasSuffix(tag, "-patched") {
			reg, _ := stringVal(m, "registry")
			// Reconstruct original ref: strip registry prefix and -patched suffix
			origTag := strings.TrimSuffix(tag, "-patched")
			origRepo := repo
			origReg := reg
			// The patched image registry is the verity registry;
			// the original registry we reconstruct from the report filename
			info := patchedImageInfo{
				Registry:   reg,
				Repository: repo,
				Tag:        tag,
				ValuesPath: path,
			}
			// Key by what sanitize(originalRef) would produce
			origRef := origRepo
			if origReg != "" {
				// Original registry is unknown here; we'll match by repo+tag
			}
			_ = origRef
			// Use repo + original tag as key for matching
			key := repo + ":" + origTag
			result[key] = info
			return
		}
	}

	for k, v := range m {
		next := k
		if path != "" {
			next = path + "." + k
		}
		collectPatchedImages(v, next, result)
	}
}

// matchReportsToImages reads Trivy JSON reports from the reports directory
// and creates SiteImage entries for each one.
func matchReportsToImages(reportsDir string, patchedImages map[string]patchedImageInfo, overrides, imagePaths map[string]string, registry, chartName string) ([]SiteImage, error) {
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var images []SiteImage
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		reportPath := filepath.Join(reportsDir, entry.Name())
		report, err := parseTrivyReportFull(reportPath)
		if err != nil {
			return nil, fmt.Errorf("parsing report %s: %w", entry.Name(), err)
		}

		// Reconstruct original ref from sanitized filename
		// Filename: quay.io_brancz_kube-rbac-proxy_v0.14.0.json
		sanitizedName := strings.TrimSuffix(entry.Name(), ".json")
		originalRef := unsanitize(sanitizedName)

		// Build patched ref
		patchedRef := buildPatchedRef(originalRef, registry)

		// Find values path from patched images map, falling back to paths.json
		valuesPath := findValuesPath(originalRef, patchedImages)
		if valuesPath == "" && imagePaths != nil {
			valuesPath = imagePaths[sanitizedName]
		}

		img := buildSiteImage(sanitizedName, originalRef, patchedRef, valuesPath, chartName, report)
		if ov, ok := overrides[sanitizedName]; ok {
			img.OverriddenFrom = ov
		}
		images = append(images, img)
	}

	sort.Slice(images, func(i, j int) bool {
		return images[i].OriginalRef < images[j].OriginalRef
	})
	return images, nil
}

// unsanitize attempts to reconstruct an image reference from a sanitized filename.
// sanitize replaces / and : with _, so we need heuristics to reverse it.
// Format: registry_path_repo_tag → registry/path/repo:tag
// The last _ before a version-like segment is the : separator.
func unsanitize(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) < 2 {
		return s
	}

	// The last part is the tag (after the colon in the original ref).
	// Find the tag by looking for version-like patterns from the end.
	// Tags typically start with v, a digit, or are "latest".
	for i := len(parts) - 1; i >= 1; i-- {
		p := parts[i]
		if looksLikeTag(p) {
			host := parts[0]
			repo := strings.Join(parts[1:i], "/")
			tag := strings.Join(parts[i:], "_") // rejoin in case tag had underscores (unlikely)
			if repo == "" {
				return host + ":" + tag
			}
			return host + "/" + repo + ":" + tag
		}
	}

	// Fallback: treat last part as tag
	host := parts[0]
	repo := strings.Join(parts[1:len(parts)-1], "/")
	tag := parts[len(parts)-1]
	if repo == "" {
		return host + ":" + tag
	}
	return host + "/" + repo + ":" + tag
}

// looksLikeTag returns true if a string looks like an image tag.
func looksLikeTag(s string) bool {
	if s == "" {
		return false
	}
	if s == "latest" {
		return true
	}
	if s[0] == 'v' || (s[0] >= '0' && s[0] <= '9') {
		return true
	}
	return false
}

// buildPatchedRef constructs the patched image reference.
func buildPatchedRef(originalRef, registry string) string {
	// Parse the original ref to get repo and tag
	img := parseRef(originalRef)

	patchedTag := img.Tag
	if patchedTag == "" {
		patchedTag = "latest"
	}
	patchedTag += "-patched"

	return registry + "/" + img.Repository + ":" + patchedTag
}

// findValuesPath tries to find the values path for an image from the patched images map.
func findValuesPath(originalRef string, patchedImages map[string]patchedImageInfo) string {
	img := parseRef(originalRef)
	key := img.Repository + ":" + img.Tag
	if info, ok := patchedImages[key]; ok {
		return info.ValuesPath
	}
	return ""
}

// parseTrivyReportFull reads and parses a full Trivy JSON report.
func parseTrivyReportFull(path string) (*trivyReportFull, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report trivyReportFull
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// buildSiteImage creates a SiteImage from a Trivy report.
func buildSiteImage(id, originalRef, patchedRef, valuesPath, chartName string, report *trivyReportFull) SiteImage {
	osInfo := ""
	if report.Metadata.OS.Family != "" {
		osInfo = report.Metadata.OS.Family
		if report.Metadata.OS.Name != "" {
			osInfo += " " + report.Metadata.OS.Name
		}
	}

	vulns := make([]SiteVuln, 0)
	severityCounts := make(map[string]int)
	fixable := 0

	for _, result := range report.Results {
		for _, v := range result.Vulnerabilities {
			vuln := SiteVuln{
				ID:               v.VulnerabilityID,
				PkgName:          v.PkgName,
				InstalledVersion: v.InstalledVersion,
				FixedVersion:     v.FixedVersion,
				Severity:         v.Severity,
				Title:            v.Title,
			}
			vulns = append(vulns, vuln)
			sev := v.Severity
			if sev == "" {
				sev = "UNKNOWN"
			}
			severityCounts[sev]++
			if v.FixedVersion != "" {
				fixable++
			}
		}
	}

	return SiteImage{
		ID:          id,
		OriginalRef: originalRef,
		PatchedRef:  patchedRef,
		ValuesPath:  valuesPath,
		OS:          osInfo,
		ChartName:   chartName,
		VulnSummary: VulnSummary{
			Total:          len(vulns),
			Fixable:        fixable,
			SeverityCounts: severityCounts,
		},
		Vulnerabilities: vulns,
	}
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// SaveStandaloneReports copies Trivy reports from PatchResults into a
// local directory. This is used during assembly before pushing to OCI.
func SaveStandaloneReports(results []*PatchResult, reportsDir string) error {
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("creating standalone reports dir: %w", err)
	}

	for _, r := range results {
		// Prefer the upstream (pre-patch) report for "before" data.
		src := r.UpstreamReportPath
		if src == "" {
			src = r.ReportPath
		}
		if src == "" {
			continue
		}
		// Use the original image ref for the filename, not the patched one.
		reportName := sanitize(r.Original.Reference()) + ".json"
		destPath := filepath.Join(reportsDir, reportName)
		if err := copyFile(src, destPath); err != nil {
			return fmt.Errorf("copying report for %s: %w", r.Original.Reference(), err)
		}
	}

	// Save override metadata for site data generation.
	if err := SaveOverrides(results, reportsDir); err != nil {
		return fmt.Errorf("saving overrides: %w", err)
	}

	return nil
}

// PushStandaloneReports pushes all standalone reports in reportsDir to
// the OCI registry as a single image artifact at:
//
//	{registry}/standalone-reports:latest
//
// Each JSON report file becomes a layer in the OCI image.
func PushStandaloneReports(reportsDir, registry string) error {
	ref := registry + "/standalone-reports:latest"

	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		return fmt.Errorf("reading reports dir: %w", err)
	}

	// Build a tar archive of the reports directory content.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(reportsDir, e.Name()))
		if err != nil {
			return fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		hdr := &tar.Header{
			Name: e.Name(),
			Mode: 0o644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}

	layer, err := tarball.LayerFromReader(&buf)
	if err != nil {
		return fmt.Errorf("creating OCI layer: %w", err)
	}

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return fmt.Errorf("building OCI image: %w", err)
	}

	if err := crane.Push(img, ref); err != nil {
		return fmt.Errorf("pushing %s: %w", ref, err)
	}
	fmt.Printf("Pushed standalone reports → %s\n", ref)
	return nil
}

// pullStandaloneReports pulls the standalone-reports artifact from OCI
// and extracts the reports into a temporary directory.
func pullStandaloneReports(registry string) (string, error) {
	ref := registry + "/standalone-reports:latest"

	img, err := crane.Pull(ref)
	if err != nil {
		return "", fmt.Errorf("pulling %s: %w", ref, err)
	}

	tmpDir, err := os.MkdirTemp("", "verity-standalone-reports-")
	if err != nil {
		return "", err
	}

	layers, err := img.Layers()
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("reading layers: %w", err)
	}

	for _, layer := range layers {
		rc, err := layer.Uncompressed()
		if err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("decompressing layer: %w", err)
		}
		err = func() error {
			defer rc.Close()
			tr := tar.NewReader(rc)
			for {
				hdr, err := tr.Next()
				if err != nil {
					break
				}
				if hdr.Typeflag != tar.TypeReg {
					continue
				}
				// Sanitize the file name to prevent Zip Slip (path traversal).
				clean := filepath.Base(hdr.Name)
				if clean == "." || clean == ".." {
					continue
				}
				dest := filepath.Join(tmpDir, clean)
				// Verify the resolved path is inside tmpDir.
				if !strings.HasPrefix(dest, filepath.Clean(tmpDir)+string(os.PathSeparator)) {
					continue
				}
				data, err := io.ReadAll(tr)
				if err != nil {
					return fmt.Errorf("reading %s from tar: %w", hdr.Name, err)
				}
				if err := os.WriteFile(dest, data, 0o644); err != nil {
					return err
				}
			}
			return nil
		}()
		if err != nil {
			os.RemoveAll(tmpDir)
			return "", err
		}
	}

	return tmpDir, nil
}

// discoverStandaloneImages reads the standalone images values file and
// pulls reports from the OCI registry standalone-reports artifact.
func discoverStandaloneImages(imagesFile, registry string) ([]SiteImage, error) {
	images, err := ParseImagesFile(imagesFile)
	if err != nil {
		return nil, err
	}

	// Pull reports from OCI.
	var reportsDir string
	if registry != "" {
		dir, err := pullStandaloneReports(registry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not pull standalone reports from OCI: %v\n", err)
		} else {
			reportsDir = dir
			defer os.RemoveAll(dir)
		}
	}

	var overrides map[string]string
	if reportsDir != "" {
		overrides = loadOverrides(reportsDir)
	}

	var siteImages []SiteImage
	for _, img := range images {
		ref := img.Reference()
		sanitizedRef := sanitize(ref)

		patchedRef := buildPatchedRef(ref, registry)

		var si SiteImage
		if reportsDir != "" {
			reportPath := filepath.Join(reportsDir, sanitizedRef+".json")
			report, err := parseTrivyReportFull(reportPath)
			if err == nil {
				si = buildSiteImage(sanitizedRef, ref, patchedRef, img.Path, "", report)
			}
		}
		if si.ID == "" {
			si = SiteImage{
				ID:              sanitizedRef,
				OriginalRef:     ref,
				PatchedRef:      patchedRef,
				ValuesPath:      img.Path,
				Vulnerabilities: make([]SiteVuln, 0),
				VulnSummary: VulnSummary{
					SeverityCounts: make(map[string]int),
				},
			}
		}
		if ov, ok := overrides[sanitizedRef]; ok {
			si.OverriddenFrom = ov
		}
		siteImages = append(siteImages, si)
	}

	return siteImages, nil
}

// computeSummary aggregates stats across all charts and standalone images.
func computeSummary(charts []SiteChart, standalone []SiteImage) SiteSummary {
	chartNames := make(map[string]bool)
	summary := SiteSummary{}

	for _, c := range charts {
		chartNames[c.Name] = true
		summary.TotalImages += len(c.Images)
		for _, img := range c.Images {
			summary.TotalVulns += img.VulnSummary.Total
			summary.FixableVulns += img.VulnSummary.Fixable
		}
	}
	summary.TotalCharts = len(chartNames)

	summary.TotalImages += len(standalone)
	for _, img := range standalone {
		summary.TotalVulns += img.VulnSummary.Total
		summary.FixableVulns += img.VulnSummary.Fixable
	}

	return summary
}
