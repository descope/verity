// Package patch wraps Copa's single-image Patch entry point. It is the
// Go-native replacement for invoking the `copa patch` CLI binary via
// subprocess. See `cmd/patch.go` for the command-line surface.
package patch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/moby/buildkit/util/progress/progressui"
	copapatch "github.com/project-copacetic/copacetic/pkg/patch"
	"github.com/project-copacetic/copacetic/pkg/types"
)

// ErrEmptyImage is returned when Config.Image is blank.
var ErrEmptyImage = errors.New("image reference is required")

// ErrEmptyTag is returned when Config.PatchedTag is blank. Copa requires a
// patched tag to push the output image.
var ErrEmptyTag = errors.New("patched tag is required")

// ErrEmptyReport is returned when Config.Report is blank. Copa requires a
// Trivy report to determine which packages to patch.
var ErrEmptyReport = errors.New("trivy report path is required")

// ErrInvalidPatchedTag is returned when Config.PatchedTag contains characters
// the OCI distribution spec forbids in tag values — most notably `@`, which
// is the digest separator. Copa concatenates the staging registry with this
// tag to form the destination reference; smuggling an `@` past us produces
// references like `ghcr.io/.../cache:cilium-…-v1.19.3@sha256-amd64` that
// copa's reference parser then rejects as "invalid reference format". We
// catch the bad value here so the failure mode is a verity-layer error with
// a clear message instead of an obscure copa parse error mid-build.
//
// Per the OCI distribution spec, tag values match the regex
// `[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}` — `@`, `:`, and whitespace are all
// disallowed. We only check `@` here because that is the historically
// observed bug source (`patch-image.sh` mis-parsing digest-pinned source
// refs); a fuller regex check is intentionally deferred to copa to avoid
// duplicating its tag-validation surface.
var ErrInvalidPatchedTag = errors.New("patched tag must not contain '@' (the OCI digest separator)")

// DefaultTimeout matches the upstream `copa patch` CLI default. Callers that
// leave Config.Timeout as zero are upgraded to this value inside toOptions;
// copa's Patch() calls context.WithTimeout(ctx, opts.Timeout) unconditionally
// and a zero duration yields an already-expired context, so we guard here.
const DefaultTimeout = 5 * time.Minute

// Config collects all inputs for a single-image patch operation. It mirrors
// the subset of copa's types.Options that verity actually uses from CI; any
// copa field not listed here is left at copa's zero-value default.
type Config struct {
	// Image is the source image reference (e.g.
	// "mirror.gcr.io/library/nginx:1.29.3"). Required: Validate rejects
	// an empty value with ErrEmptyImage.
	Image string

	// PatchedTag is the output tag that copa pushes to. The tag
	// typically includes a platform suffix (e.g. "1.29.3-linux-amd64-patched").
	// Required: Validate rejects an empty value with ErrEmptyTag.
	PatchedTag string

	// Report is the path to a Trivy JSON report for the source image.
	// Required: Validate rejects an empty value with ErrEmptyReport.
	Report string

	// PkgTypes is a comma-separated list of package ecosystems to patch.
	// Copa accepts "os", "library", or "os,library". Defaults to "os,library".
	PkgTypes string

	// LibraryPatchLevel controls how aggressively copa bumps library versions
	// for CVE fixes. One of "patch" | "minor" | "major". Defaults to "patch"
	// per copa's CLI.
	LibraryPatchLevel string

	// ToolchainPatchLevel controls how aggressively copa upgrades the Go
	// toolchain when rebuilding binaries. One of "patch" | "minor" | "major".
	ToolchainPatchLevel string

	// Push toggles pushing the patched image to the registry. If false, copa
	// produces the patched image but does not push.
	Push bool

	// BuildKitAddr is the BuildKit endpoint (e.g.
	// "buildx://copa-builder" in CI or "tcp://127.0.0.1:1234" locally).
	// Empty string lets copa auto-detect via its default socket lookup.
	BuildKitAddr string

	// Timeout bounds the entire patch operation. Values <= 0 are replaced
	// with DefaultTimeout (5m) inside toOptions; copa rejects a literal zero
	// duration as "already expired".
	Timeout time.Duration

	// Platform optionally restricts copa to a single platform (e.g.
	// "linux/amd64"). Empty means copa picks per its defaults.
	Platform string

	// GoVCSURL is the explicit Go module VCS URL used by copa's Go binary
	// rebuild path when the binary lacks embedded buildinfo (stripped /
	// distroless). Flows through to types.Options.GoVCSURL, which is
	// currently provided by a go.mod replace directive pointing at the
	// verity-org/copacetic feat/go-vcs-resolution branch (upstream copa
	// PR #1546). The replace directive is dropped once #1546 merges
	// upstream and verity re-pins to a tagged release.
	GoVCSURL string
}

// ErrNoUpdatesFound is re-exported so callers can check
// `errors.Is(err, patch.ErrNoUpdatesFound)` without pulling in copa directly.
// Copa returns this sentinel when the source image is already fully patched.
var ErrNoUpdatesFound = types.ErrNoUpdatesFound

// Validate checks that the required fields are populated and that PatchedTag
// is well-formed enough to pass copa's reference parser. See
// ErrInvalidPatchedTag for the rationale on the `@` check.
func (c *Config) Validate() error {
	if c.Image == "" {
		return ErrEmptyImage
	}
	if c.PatchedTag == "" {
		return ErrEmptyTag
	}
	if strings.Contains(c.PatchedTag, "@") {
		return fmt.Errorf("%w: %q", ErrInvalidPatchedTag, c.PatchedTag)
	}
	if c.Report == "" {
		return ErrEmptyReport
	}
	return nil
}

// defaultScanner is copa's expected scanner name. Copa's CLI wraps it with the
// same default; we duplicate it here because copa's library surface validates
// a non-empty Scanner via regex and does not apply the CLI-layer default.
const defaultScanner = "trivy"

// toOptions maps Config onto the subset of copa's types.Options that verity
// wires up. Fields left zero-valued use copa's defaults. The returned
// Timeout is always strictly positive; callers that set Timeout <= 0 get
// DefaultTimeout, because copa treats zero as an already-expired deadline.
func (c *Config) toOptions() *types.Options {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	opts := &types.Options{
		Image:               c.Image,
		PatchedTag:          c.PatchedTag,
		Report:              c.Report,
		Scanner:             defaultScanner,
		PkgTypes:            c.PkgTypes,
		LibraryPatchLevel:   c.LibraryPatchLevel,
		ToolchainPatchLevel: c.ToolchainPatchLevel,
		Push:                c.Push,
		BkAddr:              c.BuildKitAddr,
		Timeout:             timeout,
		GoVCSURL:            c.GoVCSURL,
		Progress:            progressui.DisplayMode("plain"),
	}
	if c.Platform != "" {
		opts.Platforms = []string{c.Platform}
	}
	return opts
}

// patchFunc matches copapatch.Patch's signature. Exposed as a package-level
// indirection so tests can inject a stub without touching copa or BuildKit.
var patchFunc = copapatch.Patch

// Run executes a single-image patch via copa's library API. It mirrors the
// behaviour of `copa patch …` as invoked by `.github/scripts/patch-image.sh`.
//
// A returned ErrNoUpdatesFound means the image is already clean — callers
// (notably patch-image.sh) treat this as a non-error exit code.
func Run(ctx context.Context, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid patch config: %w", err)
	}

	if err := patchFunc(ctx, cfg.toOptions()); err != nil {
		return fmt.Errorf("copa patch %s: %w", cfg.Image, err)
	}
	return nil
}
