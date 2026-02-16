package internal

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
)

type Dependency struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
	Condition  string `yaml:"condition,omitempty"`
}

type ChartFile struct {
	APIVersion   string       `yaml:"apiVersion"`
	Name         string       `yaml:"name"`
	Description  string       `yaml:"description,omitempty"`
	Type         string       `yaml:"type,omitempty"`
	Version      string       `yaml:"version"`
	Dependencies []Dependency `yaml:"dependencies"`
}

func ParseChartFile(path string) (*ChartFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cf ChartFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cf, nil
}

func DownloadChart(dep Dependency, destDir string) (string, error) {
	// Direct .tgz URL — download and extract.
	if strings.HasSuffix(dep.Repository, ".tgz") || strings.HasSuffix(dep.Repository, ".tar.gz") {
		return downloadTarball(dep.Repository, dep.Name, destDir)
	}

	// Helm SDK pull (OCI or HTTP repo).
	chartPath, err := helmPull(dep, destDir)
	if err != nil {
		return "", err
	}
	return chartPath, nil
}

func helmPull(dep Dependency, destDir string) (string, error) {
	settings := cli.New()
	cfg := &action.Configuration{}

	if strings.HasPrefix(dep.Repository, "oci://") {
		regClient, err := registry.NewClient()
		if err != nil {
			return "", fmt.Errorf("creating registry client: %w", err)
		}
		cfg.RegistryClient = regClient
	}

	pull := action.NewPullWithOpts(action.WithConfig(cfg))
	pull.Settings = settings
	pull.Untar = true
	pull.UntarDir = destDir
	pull.DestDir = destDir
	pull.Version = dep.Version

	var chartRef string
	if strings.HasPrefix(dep.Repository, "oci://") {
		chartRef = dep.Repository + "/" + dep.Name
	} else {
		pull.RepoURL = dep.Repository
		chartRef = dep.Name
	}

	_, err := pull.Run(chartRef)
	if err != nil {
		return "", fmt.Errorf("pulling %s@%s: %w", dep.Name, dep.Version, err)
	}

	return filepath.Join(destDir, dep.Name), nil
}

// downloadTarball fetches a .tgz URL and extracts it into destDir.
func downloadTarball(url, chartName, destDir string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}

	return extractTarGz(resp.Body, chartName, destDir)
}

func extractTarGz(r io.Reader, chartName, destDir string) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("gzip reader: %w", err)
	}
	defer func() {
		if err := gz.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close gzip reader: %v\n", err)
		}
	}()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(destDir, filepath.Clean(hdr.Name))

		// Prevent path traversal.
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			// Mask to valid file mode bits to prevent integer overflow
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return "", err
			}
			if err := f.Close(); err != nil {
				return "", err
			}
		}
	}

	return filepath.Join(destDir, chartName), nil
}

// PublishChart packages a chart directory and pushes it to an OCI registry.
// Returns the path to the packaged .tgz file.
func PublishChart(chartDir, registry string) (string, error) {
	// Create a temp directory for the package output
	tmpDir, err := os.MkdirTemp("", "helm-package-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	// Package the chart
	cmd := exec.Command("helm", "package", chartDir, "-d", tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("helm package failed: %w\nOutput: %s", err, output)
	}

	// Find the packaged .tgz file
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", fmt.Errorf("reading package dir: %w", err)
	}
	var tgzPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tgz") {
			tgzPath = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if tgzPath == "" {
		return "", fmt.Errorf("no .tgz file found after packaging")
	}

	// Push to OCI registry
	ociURL := fmt.Sprintf("oci://%s/charts", registry)
	cmd = exec.Command("helm", "push", tgzPath, ociURL)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("helm push failed: %w\nOutput: %s", err, output)
	}

	fmt.Printf("Published chart to %s\n", ociURL)
	return tgzPath, nil
}
