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
	patch := flag.Bool("patch", false, "scan and patch images with Trivy + Copa")
	registry := flag.String("registry", "", "target registry for patched images (e.g. ghcr.io/descope)")
	buildkitAddr := flag.String("buildkit-addr", "", "BuildKit address for Copa (e.g. docker-container://buildkitd)")
	reportDir := flag.String("report-dir", "", "directory to store Trivy JSON reports (default: temp dir)")
	siteDataPath := flag.String("site-data", "", "generate site catalog JSON at this path")
	flag.Parse()

	chart, err := internal.ParseChartFile(*chartFile)
	if err != nil {
		log.Fatalf("Failed to parse %s: %v", *chartFile, err)
	}

	// Parse image tag overrides (e.g. distroless-libc → debian) from the images file.
	var overrides []internal.ImageOverride
	if *imagesFile != "" {
		overrides, err = internal.ParseOverrides(*imagesFile)
		if err != nil {
			log.Fatalf("Failed to parse overrides from %s: %v", *imagesFile, err)
		}
		if len(overrides) > 0 {
			fmt.Printf("Loaded %d image override(s)\n", len(overrides))
			for _, o := range overrides {
				fmt.Printf("  %s: %q → %q\n", o.Repository, o.From, o.To)
			}
		}
	}

	tmpDir, err := os.MkdirTemp("", "verity-")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, dep := range chart.Dependencies {
		fmt.Printf("Processing %s@%s\n", dep.Name, dep.Version)

		chartPath, err := internal.DownloadChart(dep, tmpDir)
		if err != nil {
			log.Fatalf("Failed to download %s: %v", dep.Name, err)
		}

		images, err := internal.ScanForImages(chartPath)
		if err != nil {
			log.Fatalf("Failed to scan %s: %v", dep.Name, err)
		}

		// Apply image overrides before patching (e.g. swap distroless tags for Copa-compatible ones).
		// Track original tags so we can record overrides in results.
		origTags := make(map[string]string) // repo -> original tag
		if len(overrides) > 0 {
			for _, img := range images {
				origTags[img.Repository] = img.Tag
			}
		}
		images = internal.ApplyOverrides(images, overrides)

		fmt.Printf("  Found %d images\n", len(images))
		for _, img := range images {
			fmt.Printf("    %s  (%s)\n", img.Reference(), img.Path)
		}

		if !*patch {
			continue
		}

		// Patch mode: run Trivy + Copa on each image.
		rDir := *reportDir
		if rDir == "" {
			rDir = filepath.Join(tmpDir, "reports")
		}
		if err := os.MkdirAll(rDir, 0o755); err != nil {
			log.Fatalf("Failed to create report dir: %v", err)
		}

		opts := internal.PatchOptions{
			TargetRegistry: *registry,
			BuildKitAddr:   *buildkitAddr,
			ReportDir:      rDir,
			WorkDir:        tmpDir,
		}

		var results []*internal.PatchResult
		failed := 0
		ctx := context.Background()
		for _, img := range images {
			fmt.Printf("\n  Patching %s ...\n", img.Reference())
			r := internal.PatchImage(ctx, img, opts)

			// Record if image tag was overridden.
			if orig, ok := origTags[img.Repository]; ok && orig != img.Tag {
				r.OverriddenFrom = orig
			}

			results = append(results, r)

			if r.Error != nil {
				fmt.Printf("    ERROR: %v\n", r.Error)
				failed++
			} else if r.Skipped {
				fmt.Printf("    Skipped: %s\n", r.SkipReason)
			} else {
				fmt.Printf("    Patched → %s  (%d vulns fixed)\n", r.Patched.Reference(), r.VulnCount)
			}
		}

		if failed > 0 {
			log.Fatalf("  %d image(s) failed to patch", failed)
		}

		// Create a wrapper chart that subcharts the original with patched images
		version, err := internal.CreateWrapperChart(dep, results, *outputDir, *registry)
		if err != nil {
			log.Fatalf("Failed to create wrapper chart: %v", err)
		}
		fmt.Printf("\n  Wrapper chart → %s/%s (%s)\n", *outputDir, dep.Name, version)
	}

	// Process standalone images from values file
	if *imagesFile != "" {
		images, err := internal.ParseImagesFile(*imagesFile)
		if err != nil {
			log.Fatalf("Failed to parse images file %s: %v", *imagesFile, err)
		}

		if len(images) > 0 {
			fmt.Printf("\nStandalone images from %s:\n", *imagesFile)
			fmt.Printf("  Found %d images\n", len(images))
			for _, img := range images {
				fmt.Printf("    %s  (%s)\n", img.Reference(), img.Path)
			}

			if *patch {
				rDir := *reportDir
				if rDir == "" {
					rDir = filepath.Join(tmpDir, "reports")
				}
				if err := os.MkdirAll(rDir, 0o755); err != nil {
					log.Fatalf("Failed to create report dir: %v", err)
				}

				opts := internal.PatchOptions{
					TargetRegistry: *registry,
					BuildKitAddr:   *buildkitAddr,
					ReportDir:      rDir,
					WorkDir:        tmpDir,
				}

				failed := 0
				var results []*internal.PatchResult
				ctx := context.Background()
				for _, img := range images {
					fmt.Printf("\n  Patching %s ...\n", img.Reference())
					r := internal.PatchImage(ctx, img, opts)
					results = append(results, r)

					if r.Error != nil {
						fmt.Printf("    ERROR: %v\n", r.Error)
						failed++
					} else if r.Skipped {
						fmt.Printf("    Skipped: %s\n", r.SkipReason)
					} else {
						fmt.Printf("    Patched → %s  (%d vulns fixed)\n", r.Patched.Reference(), r.VulnCount)
					}
				}

				if failed > 0 {
					log.Fatalf("  %d standalone image(s) failed to patch", failed)
				}

				// Save standalone reports to persistent directory
				if err := internal.SaveStandaloneReports(results, "reports"); err != nil {
					log.Fatalf("Failed to save standalone reports: %v", err)
				}
			}
		}
	}

	// Generate site catalog JSON
	if *siteDataPath != "" {
		if err := internal.GenerateSiteData(*outputDir, *imagesFile, "reports", *registry, *siteDataPath); err != nil {
			log.Fatalf("Failed to generate site data: %v", err)
		}
		fmt.Printf("\nSite data → %s\n", *siteDataPath)
	}
}
