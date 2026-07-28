package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/discovery"
	"github.com/verity-org/verity/internal/integer/render"
)

var (
	errIntegerVariantNotFound = errors.New("version/type not found")
	errIntegerNoVersions      = errors.New("no versions found")
	errIntegerLatestOffline   = errors.New("`--version=latest` requires `--apkindex-url` to query Wolfi (or pass an explicit version)")
)

var integerBuildCmd = &cli.Command{
	Name:  "build",
	Usage: "Build a single image variant locally using apko (single-arch)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Aliases:  []string{"i"},
			Usage:    "Image name (e.g., node)",
			Required: true,
		},
		&cli.StringFlag{
			Name:    "version",
			Aliases: []string{"V"},
			Usage:   "Version (e.g., 22, 3.12, latest)",
			Value:   latestSentinel,
		},
		&cli.StringFlag{
			Name:    "type",
			Aliases: []string{"t"},
			Usage:   "Image type (e.g., default, dev, fips)",
			Value:   "default",
		},
		&cli.StringFlag{
			Name:  "images-dir",
			Usage: "Path to the images/ directory",
			Value: "images",
		},
		&cli.StringFlag{
			Name:  "output",
			Usage: "Output tarball path",
			Value: "image.tar",
		},
		&cli.StringFlag{
			Name:  "arch",
			Usage: "Target architecture",
			Value: "amd64",
		},
		&cli.StringFlag{
			Name:  "apkindex-url",
			Usage: "Wolfi APKINDEX URL",
			Value: apkindex.DefaultAPKINDEXURL,
		},
		&cli.StringFlag{
			Name:  "fail-on-severity",
			Usage: "Comma-separated Trivy severities that fail the build before publish (e.g. HIGH,CRITICAL). Empty disables the gate.",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		imageName := cmd.String("image")
		version := cmd.String("version")
		typeName := cmd.String("type")
		imagesDir := cmd.String("images-dir")
		apkindexURL := cmd.String("apkindex-url")

		def, err := intconfig.LoadImageByName(imagesDir, imageName)
		if err != nil {
			return fmt.Errorf("loading image %q: %w", imageName, err)
		}

		tmpl, ok := def.Types[typeName]
		if !ok {
			return fmt.Errorf("type %q not defined for image %q: %w", typeName, imageName, errIntegerVariantNotFound)
		}

		// Fail fast on the offline-`latest` ambiguity: with
		// --apkindex-url="" we can't actually consult Wolfi, so
		// "latest" would otherwise silently fall back to the highest
		// version declared in the image yaml — which is NOT what
		// `latest` is documented to mean (highest stream Wolfi
		// publishes). Force the user to either pass an explicit
		// --version or wire APKINDEX. PR #311 review thread MKoO.
		if version == latestSentinel && apkindexURL == "" {
			return fmt.Errorf("image %q: %w", imageName, errIntegerLatestOffline)
		}

		// Fetch APKINDEX only when actually needed. Pre-#311 the build
		// path never fetched, so explicit pinned versions kept working
		// offline. Restoring that contract without losing alias
		// resolution: integerBuildNeedsAPKINDEX returns true iff the
		// caller asked for `latest` OR the declared version looks like
		// a floating stream that might require aliasing AND the def
		// has a versioned package pattern. Fully-pinned versions
		// (≥ 2 dots, e.g. "1.17.5") on versioned-pattern images, and
		// any version on bespoke-only / unversioned-pattern images,
		// skip the fetch entirely. PR #311 review thread MKoM.
		var pkgs []apkindex.Package
		if apkindexURL != "" && integerBuildNeedsAPKINDEX(def, version) {
			pkgs, err = apkindex.Fetch(apkindexURL, "", 0)
			if err != nil {
				return fmt.Errorf("fetching APKINDEX: %w", err)
			}
		}

		if version == latestSentinel {
			version, err = integerResolveLatestVersionFromPkgs(def, pkgs)
			if err != nil {
				return err
			}
		} else if !slices.Contains(discovery.ResolveVersions(def, pkgs), version) {
			return fmt.Errorf("image %q version %q not defined for build: %w", imageName, version, errIntegerVariantNotFound)
		}

		// Resolve the declared stream to the actual stem render.Config will
		// substitute. For "22"-style streams that map 1:1 to a Wolfi APK
		// (`nodejs-22`) renderVersion == version. For floating-major
		// streams (`kyverno-1` → only `kyverno-1.17` is published)
		// renderVersion is the highest matching minor so apko's apk
		// solver can satisfy the constraint at publish time. Mirrors
		// `internal/integer/discovery/discover.go::expandImage` exactly
		// — see ResolveStreamRenderVersion's doc for the design.
		renderVersion := discovery.ResolveStreamRenderVersion(def, pkgs, version)
		arch := cmd.String("arch")
		melangeArtifacts, err := integerPrepareMelangeBuild(ctx, def.MelangeFor(version, typeName), renderVersion, arch)
		if err != nil {
			return fmt.Errorf("preparing melange build: %w", err)
		}
		if err := pinLocalPackageVersions(&tmpl, renderVersion, melangeArtifacts.Packages); err != nil {
			return fmt.Errorf("pinning bespoke package versions: %w", err)
		}

		tmp, err := os.CreateTemp("", "integer-build-*.apko.yaml")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
		defer os.Remove(tmp.Name())

		basePath := imagesDir + "/_base"
		out, err := render.Config(&tmpl, renderVersion, basePath)
		if err != nil {
			return fmt.Errorf("rendering apko config: %w", err)
		}
		if _, err := tmp.Write(out); err != nil {
			return fmt.Errorf("writing apko config: %w", err)
		}
		tmp.Close()

		output := cmd.String("output")
		fmt.Fprintf(os.Stderr, "Building %s:%s-%s (%s) → %s\n", imageName, version, typeName, arch, output)
		if err := integerRunApkoBuild(ctx, tmp.Name(), output, arch, melangeArtifacts.Repositories, melangeArtifacts.Keyrings); err != nil {
			return err
		}
		if sev := cmd.String("fail-on-severity"); sev != "" {
			fmt.Fprintf(os.Stderr, "Trivy gate: scanning %s for %s CVEs before publish\n", output, sev)
			return integerTrivyGate(ctx, output, sev)
		}
		return nil
	},
}

// integerResolveLatestVersionFromPkgs is the inner form used by the build
// action after it has already fetched APKINDEX (so alias resolution and
// latest-version resolution share a single fetch).
func integerResolveLatestVersionFromPkgs(def *intconfig.ImageDef, pkgs []apkindex.Package) (string, error) {
	versions := discovery.ResolveVersions(def, pkgs)
	if len(versions) == 0 {
		return "", fmt.Errorf("image %q: %w", def.Name, errIntegerNoVersions)
	}
	return versions[len(versions)-1], nil
}

// integerBuildNeedsAPKINDEX reports whether the local CLI build path
// must fetch APKINDEX for this (def, version) pair.
//
// True iff:
//
//  1. version is the `latest` sentinel — APKINDEX is required to
//     resolve "latest" to a concrete stream, OR
//  2. The def has a versioned package pattern (upstream.package or any
//     types[*].packages entry contains "{{version}}") AND the caller
//     supplied a floating stream that might need alias resolution. A
//     "floating stream" is any version with at most one dot — "1",
//     "1.17", "22", "3.9". Fully pinned versions (≥ 2 dots, e.g.
//     "1.17.5", "3.2.524") do not need aliasing in any common Wolfi
//     shape: ResolveAliasVersion would only ever look up the exact
//     literal in APKINDEX and fall through to returning the input
//     unchanged. Skipping the fetch in that case restores the pre-#311
//     offline-friendly behaviour for explicit pins.
//
// False otherwise — including bespoke-only / fully-unversioned images
// (no "{{version}}" anywhere, so aliasing is a no-op even with
// APKINDEX in hand).
//
// The dot-count heuristic matches Wolfi's actual layout: meta-stream
// packages ("<pkg>-<major>" or "<pkg>-<major>.<minor>") carry at most
// one dot in the stem; per-patch packages (which would have two dots)
// are essentially never declared as image streams. If a future image
// ever does declare a 2-dot stream that needs aliasing, the user can
// invoke discovery directly (which always fetches when --apkindex-url
// is set) — only the build path's auto-fetch is skipped, not the
// resolution itself.
func integerBuildNeedsAPKINDEX(def *intconfig.ImageDef, version string) bool {
	if version == latestSentinel {
		return true
	}
	if def.VersionedPackagePattern() == "" {
		return false
	}
	return strings.Count(version, ".") <= 1
}

func integerRunApkoBuild(ctx context.Context, configFile, output, arch string, extraRepositories, extraKeyrings []string) error {
	apkoPath, err := exec.LookPath("apko")
	if err != nil {
		return fmt.Errorf("apko not found in PATH (install via mise): %w", err)
	}
	args := []string{"build", "--arch", arch}
	for _, repo := range extraRepositories {
		args = append(args, "--repository-append", repo)
	}
	for _, keyring := range extraKeyrings {
		args = append(args, "--keyring-append", keyring)
	}
	args = append(args, configFile, "integer:local", output)
	cmd := exec.CommandContext(ctx, apkoPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apko build failed: %w", err)
	}
	return nil
}
