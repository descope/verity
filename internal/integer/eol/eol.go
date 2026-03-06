// Package eol fetches end-of-life data from endoflife.date API.
package eol

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// BaseURL is the endoflife.date API endpoint.
	BaseURL = "https://endoflife.date/api"

	// DefaultTimeout for HTTP requests.
	DefaultTimeout = 10 * time.Second
)

// ErrHTTPStatus is returned when the API returns an unexpected HTTP status.
var ErrHTTPStatus = errors.New("unexpected HTTP status")

// ErrAPIUnavailable is returned when the API has already failed and we're failing fast.
var ErrAPIUnavailable = errors.New("EOL API unavailable")

// Fetcher abstracts EOL data fetching for testability.
type Fetcher interface {
	FetchForImage(imageName string) (EOLData, error)
}

// ProductMapping maps image names to endoflife.date product IDs.
var ProductMapping = map[string]string{
	"golang":   "go",
	"node":     "nodejs",
	"python":   "python",
	"ruby":     "ruby",
	"rust":     "rust",
	"openjdk":  "openjdk", // or "oracle-jdk" for Oracle JDK
	"dotnet":   "dotnet",
	"php":      "php",
	"nginx":    "nginx",
	"redis":    "redis",
	"postgres": "postgresql",
	"mariadb":  "mariadb",
	"erlang":   "erlang",
	"haproxy":  "haproxy",
	"grafana":  "grafana",
}

// Cycle represents a single release cycle from the endoflife.date API.
// The EOL and LTS fields can be either a boolean or a date string.
type Cycle struct {
	Cycle       string `json:"cycle"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	EOL         any    `json:"eol"` // bool (false) or string ("2026-02-11")
	Latest      string `json:"latest,omitempty"`
	LTS         any    `json:"lts,omitempty"` // bool or string (date when LTS started)
}

// EOLDate returns the EOL date as a string, or empty if not EOL.
func (c *Cycle) EOLDate() string {
	switch v := c.EOL.(type) {
	case string:
		return v
	case bool:
		// false means not EOL yet
		return ""
	default:
		return ""
	}
}

// IsEOL returns true if this cycle has reached end-of-life.
func (c *Cycle) IsEOL() bool {
	eolDate := c.EOLDate()
	if eolDate == "" {
		return false
	}
	t, err := time.Parse("2006-01-02", eolDate)
	if err != nil {
		return false
	}
	return time.Now().After(t)
}

// Client fetches EOL data from endoflife.date.
type Client struct {
	httpClient *http.Client
	baseURL    string
	failedOnce bool
	failedMu   sync.RWMutex
}

// NewClient creates a new EOL client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: DefaultTimeout},
		baseURL:    BaseURL,
	}
}

// NewClientWithHTTP creates a client with custom HTTP client and base URL (for testing).
func NewClientWithHTTP(httpClient *http.Client, baseURL string) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// FetchCycles fetches all release cycles for a product.
func (c *Client) FetchCycles(product string) ([]Cycle, error) {
	c.failedMu.RLock()
	if c.failedOnce {
		c.failedMu.RUnlock()
		return nil, ErrAPIUnavailable
	}
	c.failedMu.RUnlock()

	url := fmt.Sprintf("%s/%s.json", c.baseURL, product)

	resp, err := c.httpClient.Get(url) //nolint:noctx // CLI tool, URL is constructed internally
	if err != nil {
		c.failedMu.Lock()
		c.failedOnce = true
		c.failedMu.Unlock()
		return nil, fmt.Errorf("fetching EOL data for %q: %w", product, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d for %q", ErrHTTPStatus, resp.StatusCode, product)
	}

	var cycles []Cycle
	if err := json.NewDecoder(resp.Body).Decode(&cycles); err != nil {
		return nil, fmt.Errorf("decoding EOL response for %q: %w", product, err)
	}

	return cycles, nil
}

// EOLData maps version strings to EOL dates.
type EOLData map[string]string

// FetchForImage fetches EOL data for an image name.
func (c *Client) FetchForImage(imageName string) (EOLData, error) {
	product, ok := ProductMapping[imageName]
	if !ok {
		return EOLData{}, nil
	}

	cycles, err := c.FetchCycles(product)
	if err != nil {
		return nil, err
	}

	if cycles == nil {
		return EOLData{}, nil
	}

	data := make(EOLData)
	for _, cycle := range cycles {
		// Normalize version: some products use "1.26", others use "20" or "3.12"
		version := normalizeVersion(cycle.Cycle)
		data[version] = cycle.EOLDate()
	}

	return data, nil
}

// normalizeVersion normalizes version strings for matching.
// endoflife.date uses "1.26" for Go, "20" for Node, etc.
func normalizeVersion(v string) string {
	// Remove any leading "v" if present
	return strings.TrimPrefix(v, "v")
}

// LookupEOL returns the EOL date for a version, or empty string if not found/not EOL.
func (d EOLData) LookupEOL(version string) string {
	if d == nil {
		return ""
	}
	return d[normalizeVersion(version)]
}
