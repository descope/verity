package apkindex

import (
	"archive/tar"
	"compress/gzip"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultAPKINDEXURL is the x86_64 Wolfi APKINDEX.
	DefaultAPKINDEXURL = "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz"

	// DefaultCacheMaxAge is how long cached APKINDEX data is considered fresh.
	DefaultCacheMaxAge = time.Hour
)

var (
	// ErrHTTPDownload is returned when the APKINDEX HTTP download returns a non-200 status.
	ErrHTTPDownload = errors.New("APKINDEX download failed")

	// ErrAPKINDEXNotFound is returned when no APKINDEX file is found inside the archive.
	ErrAPKINDEXNotFound = errors.New("APKINDEX file not found in archive")
)

// Fetch downloads and parses the Wolfi APKINDEX. Results are cached in cacheDir
// for maxAge. If maxAge <= 0 the cache is bypassed.
//
// Passing an empty cacheDir disables caching.
func Fetch(url, cacheDir string, maxAge time.Duration) ([]Package, error) {
	if cacheDir != "" && maxAge > 0 {
		if pkgs, ok := loadCache(cacheDir, maxAge); ok {
			return pkgs, nil
		}
	}

	pkgs, err := download(url)
	if err != nil {
		return nil, err
	}

	if cacheDir != "" {
		_ = saveCache(cacheDir, pkgs) //nolint:errcheck // best-effort; ignore write errors
	}
	return pkgs, nil
}

func download(url string) ([]Package, error) {
	resp, err := http.Get(url) //nolint:noctx,gosec // CLI tool, URL comes from config
	if err != nil {
		return nil, fmt.Errorf("downloading APKINDEX from %q: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrHTTPDownload, resp.StatusCode)
	}
	return parseTarGz(resp.Body)
}

// parseTarGz extracts the APKINDEX file from a tar.gz archive and parses it.
func parseTarGz(r io.Reader) ([]Package, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		if hdr.Name == "APKINDEX" {
			return Parse(tr)
		}
	}
	return nil, ErrAPKINDEXNotFound
}

// loadCache reads cached packages from disk if the cache file is fresh.
func loadCache(dir string, maxAge time.Duration) ([]Package, bool) {
	path := cachePath(dir)
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > maxAge {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	var pkgs []Package
	if err := gob.NewDecoder(f).Decode(&pkgs); err != nil {
		return nil, false
	}
	return pkgs, true
}

// saveCache writes packages to disk using gob encoding.
func saveCache(dir string, pkgs []Package) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "apkindex-*.tmp")
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(pkgs); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	f.Close()
	return os.Rename(f.Name(), cachePath(dir))
}

func cachePath(dir string) string {
	return filepath.Join(dir, "APKINDEX-x86_64.cache")
}
