package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

var (
	errIntegerValidationFailed    = errors.New("validation failed")
	errMissingDeclaredVersionType = errors.New("contains {{version}} but type has no declared versions to resolve")
)

// bespokePkgFile is the minimal subset of a melange yaml needed to verify the
// package name. Other fields (environment, pipeline, etc.) are intentionally
// ignored — this lives in /cmd to avoid bringing melange's own schema into
// the verity tree.
type bespokePkgFile struct {
	Package struct {
		Name string `yaml:"name"`
	} `yaml:"package"`
}

var integerValidateCmd = &cli.Command{
	Name:  "validate",
	Usage: "Schema-validate all image configs in images/",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to integer.yaml",
			Value:   "integer.yaml",
		},
		&cli.StringFlag{
			Name:  "images-dir",
			Usage: "Path to the images/ directory",
			Value: "images",
		},
		&cli.StringFlag{
			Name:  "bespoke-dir",
			Usage: "Path to the packages/bespoke/ directory; bespoke melange yamls referenced from images/ are cross-checked here",
			Value: filepath.Join("packages", "bespoke"),
		},
		&cli.StringFlag{
			Name:  "apkindex-url",
			Usage: "Wolfi APKINDEX URL; when set, verifies upstream packages exist (skipped if empty)",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "cache-dir",
			Usage: "Directory for caching APKINDEX data",
			Value: os.TempDir(),
		},
	},
	Action: runIntegerValidate,
}

func runIntegerValidate(_ context.Context, cmd *cli.Command) error {
	cfgPath := cmd.String("config")
	imagesDir := cmd.String("images-dir")
	bespokeDir := cmd.String("bespoke-dir")

	// Stat bespokeDir once. When the directory is missing, validateBespokeRefs
	// emits a single summary FAIL per affected def instead of N per-type
	// ENOENTs (which previously also double-counted with reportOrphanBespoke's
	// own summary). When the directory exists, the per-file behavior is
	// unchanged.
	bespokeDirExists := isExistingDir(bespokeDir)

	var pkgs []apkindex.Package
	if url := cmd.String("apkindex-url"); url != "" {
		var err error
		pkgs, err = apkindex.Fetch(url, cmd.String("cache-dir"), apkindex.DefaultCacheMaxAge)
		if err != nil {
			return fmt.Errorf("fetching APKINDEX: %w", err)
		}
	}

	failures := 0

	if _, err := intconfig.LoadConfig(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", cfgPath, err)
		failures++
	} else {
		fmt.Fprintf(os.Stdout, "OK   %s\n", cfgPath)
	}

	imageFiles, err := intconfig.ImageFilePaths(imagesDir)
	if err != nil {
		return fmt.Errorf("reading images directory: %w", err)
	}

	// Track which bespoke files are referenced so we can flag orphans below.
	referencedBespoke := map[string]string{} // bespoke filename → image yaml path

	checked := 0
	for _, defPath := range imageFiles {
		def, err := intconfig.LoadImage(defPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", defPath, err)
			failures++
			continue
		}
		if err := intconfig.Validate(def); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", defPath, err)
			failures++
			continue
		}

		apkFails, skip := validateAgainstAPKINDEX(def, defPath, pkgs)
		failures += apkFails
		if skip {
			continue
		}

		// Bespoke cross-checks: every type that declares melange.bespoke must
		// reference a packages/bespoke/<file>.yaml whose package.name appears
		// in the type's apko packages: list. Without this guard, a typo (or
		// a forgotten image-yaml entry) silently produces an apk that apko
		// can't resolve at publish time — exactly the failure mode of #297.
		bespokeFails := validateBespokeRefs(def, defPath, bespokeDir, referencedBespoke, bespokeDirExists)
		if bespokeFails > 0 {
			failures += bespokeFails
			continue
		}

		fmt.Fprintf(os.Stdout, "OK   %s (%d types, %d declared versions)\n",
			defPath, len(def.Types), len(def.Versions))
		checked++
	}

	// Detect orphan bespoke yamls: files in packages/bespoke/ that no image
	// references. These are dead weight and usually indicate someone added a
	// bespoke package but forgot to wire it into images/.
	if orphanFailures := reportOrphanBespoke(bespokeDir, referencedBespoke, bespokeDirExists); orphanFailures > 0 {
		failures += orphanFailures
	}

	if failures > 0 {
		return fmt.Errorf("%d error(s): %w", failures, errIntegerValidationFailed)
	}

	fmt.Fprintf(os.Stdout, "\nAll configs valid (%d images checked)\n", checked)
	return nil
}

// validateBespokeRefs verifies, for every type in def that declares
// melange.bespoke, that the referenced packages/bespoke/<file>.yaml exists
// and that its package.name is one of the entries in the type's apko
// packages: list. Each newly seen bespoke filename is recorded in referenced
// so the caller can later detect orphans. Returns the number of failures
// (and prints a FAIL line per failure to stderr).
//
// When dirExists is false, per-file reads are skipped and a single summary
// FAIL is emitted per affected def. This avoids two pathologies in the
// previous implementation: (1) N noisy "reading: open ...: no such file or
// directory" FAILs per type when the dir is absent, and (2) double-counting
// with reportOrphanBespoke's own summary failure.
func validateBespokeRefs(def *intconfig.ImageDef, defPath, bespokeDir string, referenced map[string]string, dirExists bool) int {
	failures := 0
	bespokeRefs := 0
	for typeName := range def.Types {
		tmpl := def.Types[typeName]
		if tmpl.Melange == nil || len(tmpl.Melange.Bespoke) == 0 {
			continue
		}
		for _, bespokeFile := range tmpl.Melange.Bespoke {
			resolvedFiles, rerr := resolveBespokeFiles(def, typeName, bespokeFile)
			if rerr != nil {
				fmt.Fprintf(os.Stderr, "FAIL %s type %q: bespoke %s: %v\n",
					defPath, typeName, bespokeFile, rerr)
				failures++
				continue
			}

			for _, resolvedFile := range resolvedFiles {
				referenced[resolvedFile] = defPath
				bespokeRefs++

				if !dirExists {
					continue
				}

				bespokePath := filepath.Join(bespokeDir, resolvedFile)
				pkgName, berr := readBespokePackageName(bespokePath)
				if berr != nil {
					fmt.Fprintf(os.Stderr, "FAIL %s type %q: bespoke %s: %v\n",
						defPath, typeName, resolvedFile, berr)
					failures++
					continue
				}
				if pkgName == "" {
					fmt.Fprintf(os.Stderr, "FAIL %s type %q: bespoke %s: missing package.name\n",
						defPath, typeName, resolvedFile)
					failures++
					continue
				}
				if !tmplPackageMatchesBespoke(def, typeName, tmpl.Packages, pkgName) {
					fmt.Fprintf(os.Stderr,
						"FAIL %s type %q: bespoke package.name %q not satisfiable by apko packages %v "+
							"for any declared version of this type (apko will fail with 'not in indexes' at publish time)\n",
						defPath, typeName, pkgName, tmpl.Packages)
					failures++
					continue
				}
			}
		}
	}

	if !dirExists && bespokeRefs > 0 {
		fmt.Fprintf(os.Stderr,
			"FAIL %s: bespoke-dir %s does not exist but %d type(s) reference bespoke files\n",
			defPath, bespokeDir, bespokeRefs)
		failures++
	}

	return failures
}

func resolveBespokeFiles(def *intconfig.ImageDef, typeName, bespokeFile string) ([]string, error) {
	if !strings.Contains(bespokeFile, "{{version}}") {
		return []string{bespokeFile}, nil
	}

	versions := make([]string, 0, len(def.Versions))
	for version, meta := range def.Versions {
		if slices.Contains(meta.SkipTypes, typeName) {
			continue
		}
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("%w: %q", errMissingDeclaredVersionType, typeName)
	}

	apkindex.SortVersions(versions)
	resolved := make([]string, 0, len(versions))
	seen := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		name := strings.ReplaceAll(bespokeFile, "{{version}}", version)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		resolved = append(resolved, name)
	}
	return resolved, nil
}

func tmplPackageMatchesBespoke(def *intconfig.ImageDef, typeName string, packages []string, pkgName string) bool {
	for _, pkg := range packages {
		if apkPackageName(pkg) == pkgName {
			return true
		}
		if !strings.Contains(pkg, "{{version}}") {
			continue
		}
		for version, meta := range def.Versions {
			if slices.Contains(meta.SkipTypes, typeName) {
				continue
			}
			if apkPackageName(strings.ReplaceAll(pkg, "{{version}}", version)) == pkgName {
				return true
			}
		}
	}
	return false
}

func apkPackageName(pkg string) string {
	idx := strings.IndexAny(pkg, "<>=~!")
	if idx < 0 {
		return pkg
	}
	return pkg[:idx]
}

// isExistingDir returns true iff path is a non-empty string and refers to
// an existing directory. Used to decide once, up front, whether the
// bespoke-dir is present so callers can skip per-file reads when it isn't.
func isExistingDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// readBespokePackageName parses a melange yaml and returns its package.name.
// Returns ("", err) on read/parse failures, ("", nil) if the field is absent.
func readBespokePackageName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading: %w", err)
	}
	var f bespokePkgFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return "", fmt.Errorf("parsing: %w", err)
	}
	return f.Package.Name, nil
}

// reportOrphanBespoke lists the immediate entries in bespokeDir and prints
// a FAIL line for every top-level *.yaml file that no image yaml references.
// Returns the failure count.
//
// Skips entirely when bespokeDir does not exist (dirExists is false): the
// "missing-but-referenced" case is already counted by validateBespokeRefs's
// per-def summary, and an orphan check is meaningless against a missing dir.
func reportOrphanBespoke(bespokeDir string, referenced map[string]string, dirExists bool) int {
	if !dirExists {
		return 0
	}
	entries, err := os.ReadDir(bespokeDir)
	if err != nil {
		// dirExists was true at startup, so a ReadDir error here is a
		// race or a permissions issue — treat as a hard failure.
		fmt.Fprintf(os.Stderr, "FAIL bespoke-dir %s: %v\n", bespokeDir, err)
		return 1
	}
	failures := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != yamlExt {
			continue
		}
		if _, ok := referenced[e.Name()]; !ok {
			fmt.Fprintf(os.Stderr,
				"FAIL %s: orphan bespoke yaml — no image in images/ references it via melange.bespoke\n",
				filepath.Join(bespokeDir, e.Name()))
			failures++
		}
	}
	return failures
}
