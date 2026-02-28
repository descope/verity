//go:build integration

package discovery

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"

	"github.com/verity-org/verity/internal/config"
)

// newTestRegistry creates an in-process OCI registry and returns its host address.
func newTestRegistry(t *testing.T) string {
	t.Helper()
	r := registry.New()
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// pushTag pushes a scratch image with the given tag to the test registry.
func pushTag(t *testing.T, host, repo, tag string) {
	t.Helper()
	ref := fmt.Sprintf("%s/%s:%s", host, repo, tag)
	if err := crane.Push(empty.Image, ref, crane.Insecure); err != nil {
		t.Fatalf("pushTag(%q): %v", ref, err)
	}
}

func TestFindTagsByPattern_Integration(t *testing.T) {
	host := newTestRegistry(t)

	// Push a variety of tags
	for _, tag := range []string{"1.25.0", "1.25.1", "1.26.0", "1.27.0", "latest", "1.27.0-alpine"} {
		pushTag(t, host, "library/nginx", tag)
	}

	spec := &config.ImageSpec{
		Image: host + "/library/nginx",
		Tags: config.TagStrategy{
			Strategy: "pattern",
			Pattern:  `^\d+\.\d+\.\d+$`,
			MaxTags:  2,
		},
	}

	got, err := FindTagsToPatch(spec)
	if err != nil {
		t.Fatalf("FindTagsToPatch() error = %v", err)
	}

	// Should pick the two highest semver tags: 1.26.0 and 1.27.0
	if len(got) != 2 {
		t.Fatalf("FindTagsToPatch() = %v (len=%d), want 2 tags", got, len(got))
	}
	if got[0] != "1.26.0" || got[1] != "1.27.0" {
		t.Errorf("FindTagsToPatch() = %v, want [1.26.0 1.27.0]", got)
	}
}

func TestFindTagsByPattern_WithExclusion_Integration(t *testing.T) {
	host := newTestRegistry(t)

	for _, tag := range []string{"1.25.0", "1.26.0", "1.27.0"} {
		pushTag(t, host, "library/nginx", tag)
	}

	spec := &config.ImageSpec{
		Image: host + "/library/nginx",
		Tags: config.TagStrategy{
			Strategy: "pattern",
			Pattern:  `^\d+\.\d+\.\d+$`,
			Exclude:  []string{"1.27.0"},
		},
	}

	got, err := FindTagsToPatch(spec)
	if err != nil {
		t.Fatalf("FindTagsToPatch() error = %v", err)
	}

	for _, tag := range got {
		if tag == "1.27.0" {
			t.Errorf("FindTagsToPatch() returned excluded tag %q", tag)
		}
	}
}

func TestFindTagsByLatest_Integration(t *testing.T) {
	host := newTestRegistry(t)

	for _, tag := range []string{"1.25.0", "1.26.0", "1.27.0", "latest"} {
		pushTag(t, host, "library/nginx", tag)
	}

	spec := &config.ImageSpec{
		Image: host + "/library/nginx",
		Tags: config.TagStrategy{
			Strategy: "latest",
		},
	}

	got, err := FindTagsToPatch(spec)
	if err != nil {
		t.Fatalf("FindTagsToPatch() error = %v", err)
	}

	if len(got) != 1 || got[0] != "1.27.0" {
		t.Errorf("FindTagsToPatch() = %v, want [1.27.0]", got)
	}
}

func TestFindTagsByPattern_NoMatches_Integration(t *testing.T) {
	host := newTestRegistry(t)
	pushTag(t, host, "library/nginx", "latest")

	spec := &config.ImageSpec{
		Image: host + "/library/nginx",
		Tags: config.TagStrategy{
			Strategy: "pattern",
			Pattern:  `^\d+\.\d+\.\d+$`,
		},
	}

	got, err := FindTagsToPatch(spec)
	if err != nil {
		t.Fatalf("FindTagsToPatch() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FindTagsToPatch() = %v, want []", got)
	}
}
