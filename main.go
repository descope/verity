package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/descope/verity/internal"
	"gopkg.in/yaml.v3"
)

func main() {
	chartFile := flag.String("chart", "Chart.yaml", "path to Chart.yaml")
	outputDir := flag.String("output", "charts", "output directory for per-chart image values")
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

		outDir := filepath.Join(*outputDir, dep.Name)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Fatalf("Failed to create %s: %v", outDir, err)
		}

		outFile := filepath.Join(outDir, "values.yaml")
		if err := writeImagesFile(outFile, images); err != nil {
			log.Fatalf("Failed to write %s: %v", outFile, err)
		}

		fmt.Printf("  Found %d images → %s\n", len(images), outFile)
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

		// Write a Helm values override with patched image refs.
		overrideFile := filepath.Join(outDir, "patched-values.yaml")
		if err := internal.GenerateValuesOverride(results, overrideFile); err != nil {
			log.Fatalf("Failed to write %s: %v", overrideFile, err)
		}
		fmt.Printf("\n  Values override → %s\n", overrideFile)

		if failed > 0 {
			log.Fatalf("  %d image(s) failed to patch", failed)
		}
	}
}

type imagesFile struct {
	Images []internal.Image `yaml:"images"`
}

func writeImagesFile(path string, images []internal.Image) error {
	data, err := yaml.Marshal(&imagesFile{Images: images})
	if err != nil {
		return fmt.Errorf("marshaling images: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
