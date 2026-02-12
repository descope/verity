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
	flag.Parse()

	chart, err := internal.ParseChartFile(*chartFile)
	if err != nil {
		log.Fatalf("Failed to parse %s: %v", *chartFile, err)
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
			results = append(results, r)

			if r.Error != nil {
				fmt.Printf("    ERROR: %v\n", r.Error)
				failed++
			} else if r.Skipped {
				fmt.Printf("    No patchable OS vulnerabilities, skipped\n")
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
				ctx := context.Background()
				for _, img := range images {
					fmt.Printf("\n  Patching %s ...\n", img.Reference())
					r := internal.PatchImage(ctx, img, opts)

					if r.Error != nil {
						fmt.Printf("    ERROR: %v\n", r.Error)
						failed++
					} else if r.Skipped {
						fmt.Printf("    No patchable OS vulnerabilities, skipped\n")
					} else {
						fmt.Printf("    Patched → %s  (%d vulns fixed)\n", r.Patched.Reference(), r.VulnCount)
					}
				}

				if failed > 0 {
					log.Fatalf("  %d standalone image(s) failed to patch", failed)
				}
			}
		}
	}
}

