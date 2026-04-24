package patch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/project-copacetic/copacetic/pkg/types"
)

// errTestCopaBoom is a package-level sentinel used by the "copa returned an
// error" test case. err113 forbids constructing errors.New(...) inline.
var errTestCopaBoom = errors.New("copa boom")

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name: "all required fields populated",
			cfg: Config{
				Image:      "nginx:1.25",
				PatchedTag: "1.25-patched",
				Report:     "trivy.json",
			},
			wantErr: nil,
		},
		{
			name: "empty image",
			cfg: Config{
				PatchedTag: "1.25-patched",
				Report:     "trivy.json",
			},
			wantErr: ErrEmptyImage,
		},
		{
			name: "empty tag",
			cfg: Config{
				Image:  "nginx:1.25",
				Report: "trivy.json",
			},
			wantErr: ErrEmptyTag,
		},
		{
			name: "empty report",
			cfg: Config{
				Image:      "nginx:1.25",
				PatchedTag: "1.25-patched",
			},
			wantErr: ErrEmptyReport,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestToOptionsFieldMapping(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Image:               "mirror.gcr.io/library/nginx:1.29.3",
		PatchedTag:          "1.29.3-linux-amd64-patched",
		Report:              "reports/nginx.json",
		PkgTypes:            "os,library",
		LibraryPatchLevel:   "major",
		ToolchainPatchLevel: "patch",
		Push:                true,
		BuildKitAddr:        "buildx://copa-builder",
		Timeout:             30 * time.Minute,
		Platform:            "linux/amd64",
		GoVCSURL:            "https://github.com/prometheus/prometheus",
	}

	opts := cfg.toOptions()

	if opts.Image != cfg.Image {
		t.Errorf("opts.Image = %q, want %q", opts.Image, cfg.Image)
	}
	if opts.PatchedTag != cfg.PatchedTag {
		t.Errorf("opts.PatchedTag = %q, want %q", opts.PatchedTag, cfg.PatchedTag)
	}
	if opts.Report != cfg.Report {
		t.Errorf("opts.Report = %q, want %q", opts.Report, cfg.Report)
	}
	if opts.Scanner != defaultScanner {
		t.Errorf("opts.Scanner = %q, want %q (hardcoded default — copa requires non-empty)",
			opts.Scanner, defaultScanner)
	}
	if opts.PkgTypes != cfg.PkgTypes {
		t.Errorf("opts.PkgTypes = %q, want %q", opts.PkgTypes, cfg.PkgTypes)
	}
	if opts.LibraryPatchLevel != cfg.LibraryPatchLevel {
		t.Errorf("opts.LibraryPatchLevel = %q, want %q", opts.LibraryPatchLevel, cfg.LibraryPatchLevel)
	}
	if opts.ToolchainPatchLevel != cfg.ToolchainPatchLevel {
		t.Errorf("opts.ToolchainPatchLevel = %q, want %q",
			opts.ToolchainPatchLevel, cfg.ToolchainPatchLevel)
	}
	if opts.Push != cfg.Push {
		t.Errorf("opts.Push = %v, want %v", opts.Push, cfg.Push)
	}
	if opts.BkAddr != cfg.BuildKitAddr {
		t.Errorf("opts.BkAddr = %q, want %q", opts.BkAddr, cfg.BuildKitAddr)
	}
	if opts.Timeout != cfg.Timeout {
		t.Errorf("opts.Timeout = %v, want %v", opts.Timeout, cfg.Timeout)
	}
	if len(opts.Platforms) != 1 || opts.Platforms[0] != cfg.Platform {
		t.Errorf("opts.Platforms = %v, want [%q]", opts.Platforms, cfg.Platform)
	}
	if opts.GoVCSURL != cfg.GoVCSURL {
		t.Errorf("opts.GoVCSURL = %q, want %q", opts.GoVCSURL, cfg.GoVCSURL)
	}
}

func TestToOptionsPlatformOmittedWhenBlank(t *testing.T) {
	t.Parallel()

	cfg := &Config{Timeout: time.Minute}
	opts := cfg.toOptions()
	if len(opts.Platforms) != 0 {
		t.Errorf("opts.Platforms = %v, want empty when Config.Platform is blank", opts.Platforms)
	}
}

// TestToOptionsAppliesTimeoutDefault guards against the bug where copa's
// context.WithTimeout(ctx, 0) fires Done() immediately and returns
// "patch exceeded timeout 0s". Any Config with Timeout <= 0 must end up
// with a strictly positive opts.Timeout.
func TestToOptionsAppliesTimeoutDefault(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   time.Duration
	}{
		{"zero", 0},
		{"negative", -1 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{Timeout: tc.in}
			opts := cfg.toOptions()
			if opts.Timeout != DefaultTimeout {
				t.Errorf("opts.Timeout = %v, want DefaultTimeout (%v)", opts.Timeout, DefaultTimeout)
			}
		})
	}
}

func TestDefaultTimeoutIsFiveMinutes(t *testing.T) {
	t.Parallel()
	if DefaultTimeout != 5*time.Minute {
		t.Errorf("DefaultTimeout = %v, want 5m (matches legacy `copa patch`)", DefaultTimeout)
	}
}

func TestErrNoUpdatesFoundReExport(t *testing.T) {
	t.Parallel()
	if !errors.Is(ErrNoUpdatesFound, types.ErrNoUpdatesFound) {
		t.Error("ErrNoUpdatesFound must be an exact re-export of types.ErrNoUpdatesFound so errors.Is works across package boundaries")
	}
}

func TestRunValidationFails(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		PatchedTag: "bar",
		Report:     "r.json",
	}
	err := Run(context.Background(), cfg)
	if !errors.Is(err, ErrEmptyImage) {
		t.Errorf("Run() = %v, want error wrapping ErrEmptyImage", err)
	}
}

// Tests that swap the package-level patchFunc must NOT run in parallel with
// each other or with Run (which reads that shared function variable).
// Running sequentially avoids a data race; the test body is short so
// serialization costs nothing meaningful.

func TestRunCallsPatchFunc(t *testing.T) {
	var gotOpts *types.Options
	stub := func(_ context.Context, opts *types.Options) error {
		gotOpts = opts
		return nil
	}
	restore := withPatchFunc(t, stub)
	defer restore()

	cfg := &Config{
		Image:      "alpine:3.18",
		PatchedTag: "alpine-patched",
		Report:     "trivy.json",
		Timeout:    time.Minute,
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if gotOpts == nil {
		t.Fatal("patchFunc was not invoked")
	}
	if gotOpts.Image != "alpine:3.18" {
		t.Errorf("patchFunc received Image %q, want %q", gotOpts.Image, "alpine:3.18")
	}
}

func TestRunWrapsCopaError(t *testing.T) {
	stub := func(_ context.Context, _ *types.Options) error { return errTestCopaBoom }
	restore := withPatchFunc(t, stub)
	defer restore()

	cfg := &Config{
		Image:      "alpine:3.18",
		PatchedTag: "alpine-patched",
		Report:     "trivy.json",
	}
	err := Run(context.Background(), cfg)
	if !errors.Is(err, errTestCopaBoom) {
		t.Errorf("Run() = %v, want wrapped errTestCopaBoom", err)
	}
}

func TestRunPropagatesErrNoUpdatesFound(t *testing.T) {
	stub := func(_ context.Context, _ *types.Options) error { return types.ErrNoUpdatesFound }
	restore := withPatchFunc(t, stub)
	defer restore()

	cfg := &Config{
		Image:      "alpine:3.18",
		PatchedTag: "alpine-patched",
		Report:     "trivy.json",
	}
	err := Run(context.Background(), cfg)
	if !errors.Is(err, ErrNoUpdatesFound) {
		t.Errorf("Run() = %v, want error wrapping ErrNoUpdatesFound", err)
	}
}

// TestRunPlumbsGoVCSURLToCopa guards the migration-window contract: the
// --go-vcs-url flag must reach copa's types.Options.GoVCSURL. Before the
// go.mod replace directive → verity-org/copacetic feat/go-vcs-resolution
// (upstream PR #1546), copa's upstream Options struct lacked this field
// and the value was silently dropped, breaking Go CVE patching for every
// image with goVcsUrl in copa-config.yaml (cert-manager, loki, consul,
// prometheus, …).
func TestRunPlumbsGoVCSURLToCopa(t *testing.T) {
	var gotOpts *types.Options
	stub := func(_ context.Context, opts *types.Options) error {
		gotOpts = opts
		return nil
	}
	restore := withPatchFunc(t, stub)
	defer restore()

	wantURL := "https://github.com/prometheus/prometheus"
	cfg := &Config{
		Image:      "quay.io/prometheus/prometheus:v3.9.1",
		PatchedTag: "v3.9.1-patched",
		Report:     "trivy.json",
		GoVCSURL:   wantURL,
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if gotOpts == nil {
		t.Fatal("patchFunc was not invoked")
	}
	if gotOpts.GoVCSURL != wantURL {
		t.Errorf("opts.GoVCSURL = %q, want %q (upstream PR #1546 carry-patch must plumb this through)",
			gotOpts.GoVCSURL, wantURL)
	}
}

// withPatchFunc temporarily replaces the package-level patchFunc with stub
// and returns a restore func. t is carried for Helper() and future t.Cleanup.
func withPatchFunc(t *testing.T, stub func(context.Context, *types.Options) error) func() {
	t.Helper()
	orig := patchFunc
	patchFunc = stub
	return func() { patchFunc = orig }
}
