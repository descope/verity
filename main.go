package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/descope/verity/internal"
)

func main() {
	chartFile := flag.String("chart", "Chart.yaml", "path to Chart.yaml")
	imagesFile := flag.String("images", "", "path to standalone images values.yaml")
	outputDir := flag.String("output", "charts", "output directory for wrapper charts")
	registry := flag.String("registry", "", "target registry for patched images (e.g. ghcr.io/descope)")
	buildkitAddr := flag.String("buildkit-addr", "", "BuildKit address for Copa (e.g. docker-container://buildkitd)")
	reportDir := flag.String("report-dir", "", "directory to store Trivy JSON reports (default: temp dir)")
	siteDataPath := flag.String("site-data", "", "generate site catalog JSON at this path")

	// Mode flags (mutually exclusive)
	discover := flag.Bool("discover", false, "discover images and output GitHub Actions matrix JSON")
	discoverDir := flag.String("discover-dir", ".verity", "output directory for discover artifacts")
	patchSingle := flag.Bool("patch-single", false, "patch a single image (for matrix jobs)")
	image := flag.String("image", "", "image reference to patch (used with -patch-single)")
	resultDir := flag.String("result-dir", "", "directory to write patch result JSON (used with -patch-single)")
	assemble := flag.Bool("assemble", false, "assemble wrapper charts from matrix job results")
	manifestPath := flag.String("manifest", "", "path to manifest.json (used with -assemble)")
	resultsDir := flag.String("results-dir", "", "directory containing patch result JSONs (used with -assemble)")
	reportsDir := flag.String("reports-dir", "", "directory containing Trivy reports (used with -assemble)")

	// Scan-only mode (no patching)
	scan := flag.Bool("scan", false, "scan charts for images without patching (dry run)")
	flag.Parse()

	// Validate mutual exclusivity of mode flags.
	modeCount := 0
	for _, set := range []bool{*discover, *patchSingle, *assemble, *scan, *siteDataPath != ""} {
		if set {
			modeCount++
		}
	}
	if modeCount > 1 {
		log.Fatal("Only one mode flag may be specified at a time (-discover, -patch-single, -assemble, -scan, -site-data)")
	}

	switch {
	case *discover:
		runDiscover(*chartFile, *imagesFile, *discoverDir)
	case *patchSingle:
		runPatchSingle(*image, *registry, *buildkitAddr, *reportDir, *resultDir)
	case *assemble:
		runAssemble(*manifestPath, *resultsDir, *reportsDir, *outputDir, *registry, *imagesFile, *siteDataPath)
	case *scan:
		runScan(*chartFile, *imagesFile)
	case *siteDataPath != "":
		runSiteData(*outputDir, *imagesFile, *registry, *siteDataPath)
	default:
		flag.Usage()
		fmt.Fprintf(os.Stderr, "\nModes:\n")
		fmt.Fprintf(os.Stderr, "  -discover       Scan charts and output a GitHub Actions matrix\n")
		fmt.Fprintf(os.Stderr, "  -patch-single   Patch a single image (run in a matrix job)\n")
		fmt.Fprintf(os.Stderr, "  -assemble       Assemble wrapper charts from matrix results\n")
		fmt.Fprintf(os.Stderr, "  -scan           List images found in charts (dry run)\n")
		fmt.Fprintf(os.Stderr, "  -site-data      Generate site catalog JSON from existing charts\n")
		os.Exit(1)
	}
}

// runDiscover scans charts and standalone images, then writes a manifest
// and a GitHub Actions matrix JSON to discoverDir.
func runDiscover(chartFile, imagesFile, discoverDir string) {
	tmpDir, err := os.MkdirTemp("", "verity-discover-")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	manifest, err := internal.DiscoverImages(chartFile, imagesFile, tmpDir)
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	matrix := internal.GenerateMatrix(manifest)

	if err := internal.WriteDiscoveryOutput(manifest, matrix, discoverDir); err != nil {
		log.Fatalf("Failed to write discovery output: %v", err)
	}

	fmt.Printf("\nDiscovery complete: %d unique images\n", len(matrix.Include))
	fmt.Printf("  Manifest → %s/manifest.json\n", discoverDir)
	fmt.Printf("  Matrix   → %s/matrix.json\n", discoverDir)
}

// runPatchSingle patches a single image and writes the result. Designed
// to run inside a GitHub Actions matrix job.
func runPatchSingle(imageRef, registry, buildkitAddr, reportDir, resultDir string) {
	if imageRef == "" {
		log.Fatal("-image is required with -patch-single")
	}
	if resultDir == "" {
		log.Fatal("-result-dir is required with -patch-single")
	}

	tmpDir, err := os.MkdirTemp("", "verity-patch-")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	rDir := reportDir
	if rDir == "" {
		rDir = filepath.Join(tmpDir, "reports")
	}

	opts := internal.PatchOptions{
		TargetRegistry: registry,
		BuildKitAddr:   buildkitAddr,
		ReportDir:      rDir,
		WorkDir:        tmpDir,
	}

	fmt.Printf("Patching %s ...\n", imageRef)
	ctx := context.Background()
	if err := internal.PatchSingleImage(ctx, imageRef, opts, resultDir); err != nil {
		log.Fatalf("Patch failed: %v", err)
	}
	fmt.Println("Done.")
}

// runAssemble reads the discovery manifest and matrix job results, then
// creates wrapper charts and optionally generates site data.
func runAssemble(manifestPath, resultsDir, reportsDir, outputDir, registry, imagesFile, siteDataPath string) {
	if manifestPath == "" {
		log.Fatal("-manifest is required with -assemble")
	}
	if resultsDir == "" {
		log.Fatal("-results-dir is required with -assemble")
	}
	if reportsDir == "" {
		log.Fatal("-reports-dir is required with -assemble")
	}

	fmt.Println("Assembling wrapper charts from matrix results ...")
	if err := internal.AssembleResults(manifestPath, resultsDir, reportsDir, outputDir, registry); err != nil {
		log.Fatalf("Assembly failed: %v", err)
	}

	if siteDataPath != "" {
		if err := internal.GenerateSiteData(outputDir, imagesFile, "reports", registry, siteDataPath); err != nil {
			log.Fatalf("Failed to generate site data: %v", err)
		}
		fmt.Printf("Site data → %s\n", siteDataPath)
	}

	fmt.Println("Assembly complete.")
}

// runSiteData generates the site catalog JSON from existing charts and reports.
func runSiteData(outputDir, imagesFile, registry, siteDataPath string) {
	if err := internal.GenerateSiteData(outputDir, imagesFile, "reports", registry, siteDataPath); err != nil {
		log.Fatalf("Failed to generate site data: %v", err)
	}
	fmt.Printf("Site data → %s\n", siteDataPath)
}

// runScan is a lightweight dry-run mode that lists all images found
// in charts and standalone values without patching anything.
func runScan(chartFile, imagesFile string) {
	tmpDir, err := os.MkdirTemp("", "verity-scan-")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	chart, err := internal.ParseChartFile(chartFile)
	if err != nil {
		log.Fatalf("Failed to parse %s: %v", chartFile, err)
	}

	total := 0
	for _, dep := range chart.Dependencies {
		fmt.Printf("Chart %s@%s\n", dep.Name, dep.Version)

		chartPath, err := internal.DownloadChart(dep, tmpDir)
		if err != nil {
			log.Fatalf("Failed to download %s: %v", dep.Name, err)
		}

		images, err := internal.ScanForImages(chartPath)
		if err != nil {
			log.Fatalf("Failed to scan %s: %v", dep.Name, err)
		}

		for _, img := range images {
			fmt.Printf("  %s  (%s)\n", img.Reference(), img.Path)
		}
		total += len(images)
	}

	if imagesFile != "" {
		images, err := internal.ParseImagesFile(imagesFile)
		if err != nil {
			log.Fatalf("Failed to parse %s: %v", imagesFile, err)
		}
		fmt.Printf("Standalone images from %s\n", imagesFile)
		for _, img := range images {
			fmt.Printf("  %s  (%s)\n", img.Reference(), img.Path)
		}
		total += len(images)
	}

	fmt.Printf("\nTotal: %d images\n", total)
}
