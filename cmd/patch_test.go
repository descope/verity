package cmd

import (
	"errors"
	"testing"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/patch"
)

func TestPatchConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     patch.Config
		wantErr error
	}{
		{
			name: "all required fields",
			cfg: patch.Config{
				Image:      "nginx:1.25",
				PatchedTag: "1.25-patched",
				Report:     "trivy.json",
			},
			wantErr: nil,
		},
		{
			name: "missing image",
			cfg: patch.Config{
				PatchedTag: "1.25-patched",
				Report:     "trivy.json",
			},
			wantErr: patch.ErrEmptyImage,
		},
		{
			name: "missing tag",
			cfg: patch.Config{
				Image:  "nginx:1.25",
				Report: "trivy.json",
			},
			wantErr: patch.ErrEmptyTag,
		},
		{
			name: "missing report",
			cfg: patch.Config{
				Image:      "nginx:1.25",
				PatchedTag: "1.25-patched",
			},
			wantErr: patch.ErrEmptyReport,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPatchCommandFlagNames(t *testing.T) {
	t.Parallel()

	want := []string{
		"image", "tag", "report", "pkg-types",
		"library-patch-level", "toolchain-patch-level",
		"push", "buildkit-addr", "timeout", "platform", "go-vcs-url",
	}

	got := make(map[string]bool, len(PatchCommand.Flags))
	for _, f := range PatchCommand.Flags {
		for _, name := range f.Names() {
			got[name] = true
		}
	}

	for _, flag := range want {
		if !got[flag] {
			t.Errorf("PatchCommand is missing required flag %q", flag)
		}
	}
}

func TestPatchCommandShortAliases(t *testing.T) {
	t.Parallel()

	wantAliases := map[string]string{
		"i": "image",
		"t": "tag",
		"r": "report",
	}

	for short, long := range wantAliases {
		found := false
		for _, f := range PatchCommand.Flags {
			names := f.Names()
			hasShort := false
			hasLong := false
			for _, n := range names {
				if n == short {
					hasShort = true
				}
				if n == long {
					hasLong = true
				}
			}
			if hasShort && hasLong {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PatchCommand missing short alias -%s for --%s", short, long)
		}
	}
}

// TestPatchCommandDefaults locks in non-zero defaults for flags where the
// zero value would cause copa to misbehave. A zero --timeout in particular
// makes copa construct context.WithTimeout(ctx, 0), which fires Done()
// immediately and returns "patch exceeded timeout 0s" before doing work.
func TestPatchCommandDefaults(t *testing.T) {
	t.Parallel()

	var timeoutFlag *cli.DurationFlag
	for _, f := range PatchCommand.Flags {
		if df, ok := f.(*cli.DurationFlag); ok && df.Name == "timeout" {
			timeoutFlag = df
			break
		}
	}
	if timeoutFlag == nil {
		t.Fatalf("PatchCommand is missing the --timeout flag")
	}
	if timeoutFlag.Value <= 0 {
		t.Errorf("--timeout default = %v, want > 0 (copa interprets 0 as 'already expired')", timeoutFlag.Value)
	}

	wantStringDefaults := map[string]string{
		"pkg-types":             "os,library",
		"library-patch-level":   "patch",
		"toolchain-patch-level": "patch",
	}
	for _, f := range PatchCommand.Flags {
		sf, ok := f.(*cli.StringFlag)
		if !ok {
			continue
		}
		want, tracked := wantStringDefaults[sf.Name]
		if !tracked {
			continue
		}
		if sf.Value != want {
			t.Errorf("--%s default = %q, want %q", sf.Name, sf.Value, want)
		}
	}

	// Keep defaultPatchTimeout stable — callers (tests, downstream tools)
	// may rely on 5m specifically.
	if defaultPatchTimeout != 5*time.Minute {
		t.Errorf("defaultPatchTimeout = %v, want 5m (matches legacy `copa patch`)", defaultPatchTimeout)
	}
}
