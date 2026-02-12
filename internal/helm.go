package internal

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
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
	defer resp.Body.Close()

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
	defer gz.Close()

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
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return "", err
			}
			f.Close()
		}
	}

	return filepath.Join(destDir, chartName), nil
}
