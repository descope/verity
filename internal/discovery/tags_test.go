package discovery

import (
	"errors"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

func TestExcludeTags(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		exclusions []string
		want       []string
	}{
		{
			name:       "no exclusions returns all tags",
			tags:       []string{"1.0.0", "1.1.0", "1.2.0"},
			exclusions: nil,
			want:       []string{"1.0.0", "1.1.0", "1.2.0"},
		},
		{
			name:       "excludes matching tags",
			tags:       []string{"1.0.0", "1.1.0", "1.2.0"},
			exclusions: []string{"1.1.0"},
			want:       []string{"1.0.0", "1.2.0"},
		},
		{
			name:       "empty exclusions returns all tags",
			tags:       []string{"1.0.0"},
			exclusions: []string{},
			want:       []string{"1.0.0"},
		},
		{
			name:       "all tags excluded",
			tags:       []string{"1.0.0"},
			exclusions: []string{"1.0.0"},
			want:       []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExcludeTags(tc.tags, tc.exclusions)
			if len(got) != len(tc.want) {
				t.Fatalf("ExcludeTags() = %v, want %v", got, tc.want)
			}
			for i, g := range got {
				if g != tc.want[i] {
					t.Errorf("ExcludeTags()[%d] = %q, want %q", i, g, tc.want[i])
				}
			}
		})
	}
}

func TestFindTagsToPatch_List(t *testing.T) {
	spec := &config.ImageSpec{
		Image: "docker.io/library/nginx",
		Tags: config.TagStrategy{
			Strategy: "list",
			List:     []string{"1.25.3", "1.26.0"},
		},
	}

	got, err := FindTagsToPatch(spec)
	if err != nil {
		t.Fatalf("FindTagsToPatch() error = %v", err)
	}
	if len(got) != 2 || got[0] != "1.25.3" || got[1] != "1.26.0" {
		t.Errorf("FindTagsToPatch() = %v, want [1.25.3 1.26.0]", got)
	}
}

func TestFindTagsToPatch_UnknownStrategy(t *testing.T) {
	spec := &config.ImageSpec{
		Image: "docker.io/library/nginx",
		Tags: config.TagStrategy{
			Strategy: "bogus",
		},
	}

	_, err := FindTagsToPatch(spec)
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
	if !errors.Is(err, ErrUnknownStrategy) {
		t.Errorf("expected ErrUnknownStrategy, got %v", err)
	}
}

func TestTagsToSortedVersions(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{
			name: "sorts semver ascending",
			tags: []string{"1.2.0", "1.0.0", "1.1.0"},
			want: []string{"1.0.0", "1.1.0", "1.2.0"},
		},
		{
			name: "handles prefixed versions",
			tags: []string{"v3.2.1", "v3.0.0", "v3.1.0"},
			want: []string{"3.0.0", "3.1.0", "3.2.1"},
		},
		{
			name: "skips non-semver tags",
			tags: []string{"1.0.0", "latest", "nightly", "2.0.0"},
			want: []string{"1.0.0", "2.0.0"},
		},
		{
			name: "empty input",
			tags: []string{},
			want: []string{},
		},
		{
			name: "all non-semver",
			tags: []string{"latest", "edge", "nightly"},
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			versions := tagsToSortedVersions(tc.tags)
			got := make([]string, len(versions))
			for i, v := range versions {
				got[i] = v.String()
			}
			if len(got) != len(tc.want) {
				t.Fatalf("tagsToSortedVersions() returned %d versions, want %d: got %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("tagsToSortedVersions()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSelectPatternTags_NonSemverFallback(t *testing.T) {
	got := selectPatternTags(
		[]string{"unprivileged-oss-20260302", "unprivileged-oss-20260316", "unprivileged-oss-20260413"},
		nil,
		2,
	)

	want := []string{"unprivileged-oss-20260316", "unprivileged-oss-20260413"}
	if len(got) != len(want) {
		t.Fatalf("selectPatternTags() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("selectPatternTags()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCraneOptions(t *testing.T) {
	tests := []struct {
		image     string
		wantCount int
	}{
		{"ghcr.io/verity-org/nginx:latest", 0},
		{"docker.io/library/nginx:1.27", 0},
		{"localhost/myimage:v1", 1},
		{"localhost:5000/myimage:v1", 1},
		{"127.0.0.1/myimage:v1", 1},
		{"127.0.0.1:5000/myimage:v1", 1},
	}
	for _, tc := range tests {
		t.Run(tc.image, func(t *testing.T) {
			opts := craneOptions(tc.image)
			if len(opts) != tc.wantCount {
				t.Errorf("craneOptions(%q) returned %d options, want %d", tc.image, len(opts), tc.wantCount)
			}
		})
	}
}
