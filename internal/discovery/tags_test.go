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
