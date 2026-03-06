package cmd

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

var integerSyncCmd = &cli.Command{
	Name:  "sync",
	Usage: "Fetch APKINDEX and report new/stale versions; --apply updates image files",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "images-dir",
			Usage: "Path to the images/ directory",
			Value: "images",
		},
		&cli.StringFlag{
			Name:  "apkindex-url",
			Usage: "Wolfi APKINDEX URL",
			Value: apkindex.DefaultAPKINDEXURL,
		},
		&cli.StringFlag{
			Name:  "cache-dir",
			Usage: "Directory for caching APKINDEX data",
			Value: os.TempDir(),
		},
		&cli.BoolFlag{
			Name:  "apply",
			Usage: "Write new versions back into image YAML files",
		},
	},
	Action: runIntegerSync,
}

func runIntegerSync(c *cli.Context) error {
	imagesDir := c.String("images-dir")
	apply := c.Bool("apply")

	var pkgs []apkindex.Package
	if url := c.String("apkindex-url"); url != "" {
		var err error
		pkgs, err = apkindex.Fetch(url, c.String("cache-dir"), apkindex.DefaultCacheMaxAge)
		if err != nil {
			return fmt.Errorf("fetching APKINDEX: %w", err)
		}
	}

	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return fmt.Errorf("reading images directory: %w", err)
	}

	totalNew, totalStale := 0, 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		n, s := integerProcessSyncEntry(entry, imagesDir, pkgs, apply)
		totalNew += n
		totalStale += s
	}

	fmt.Fprintf(os.Stdout, "\nSummary: %d new, %d stale\n", totalNew, totalStale)
	return nil
}

func integerProcessSyncEntry(entry os.DirEntry, imagesDir string, pkgs []apkindex.Package, apply bool) (newCount, staleCount int) {
	defPath := filepath.Join(imagesDir, entry.Name())
	def, err := intconfig.LoadImage(defPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN %s: %v\n", entry.Name(), err)
		return 0, 0
	}

	discovered := apkindex.DiscoverVersions(pkgs, def.Upstream.Package)

	discoveredSet := make(map[string]bool, len(discovered))
	for _, v := range discovered {
		discoveredSet[v] = true
	}

	knownSet := make(map[string]bool, len(def.Versions))
	for v := range def.Versions {
		knownSet[v] = true
	}

	var newVersions, staleVersions []string
	for _, v := range discovered {
		if !knownSet[v] {
			newVersions = append(newVersions, v)
		}
	}
	for v := range def.Versions {
		if !discoveredSet[v] && v != "latest" {
			staleVersions = append(staleVersions, v)
		}
	}
	sort.Strings(newVersions)
	sort.Strings(staleVersions)

	integerPrintSyncReport(def.Name, newVersions, staleVersions)

	if apply && len(newVersions) > 0 {
		if err := integerApplySyncUpdates(defPath, def, newVersions); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR applying updates to %s: %v\n", entry.Name(), err)
		}
	}

	return len(newVersions), len(staleVersions)
}

func integerPrintSyncReport(name string, newVersions, staleVersions []string) {
	if len(newVersions) == 0 && len(staleVersions) == 0 {
		fmt.Fprintf(os.Stdout, "%s: up to date\n", name)
		return
	}
	fmt.Fprintf(os.Stdout, "%s:\n", name)
	if len(newVersions) > 0 {
		fmt.Fprintf(os.Stdout, "  new:   %s\n", strings.Join(newVersions, ", "))
	}
	if len(staleVersions) > 0 {
		fmt.Fprintf(os.Stdout, "  stale: %s\n", strings.Join(staleVersions, ", "))
	}
}

func integerApplySyncUpdates(path string, def *intconfig.ImageDef, newVersions []string) error {
	updated := *def
	versions := make(map[string]intconfig.VersionMeta, len(def.Versions)+len(newVersions))
	maps.Copy(versions, def.Versions)
	for _, v := range newVersions {
		versions[v] = intconfig.VersionMeta{}
	}
	updated.Versions = versions

	data, err := yaml.Marshal(&updated)
	if err != nil {
		return fmt.Errorf("marshalling: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(os.Stdout, "  applied: added %s\n", strings.Join(newVersions, ", "))
	return nil
}
