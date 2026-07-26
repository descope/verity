package discovery

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/config"
)

func Test_FindTagsToPatch_returns_highest_semver_tags_when_pattern_limit_crosses_boundary(t *testing.T) {
	// Given
	host := newTagBoundaryRegistry(t)
	pushTagBoundaryImages(t, host, "apps/sentinel", "1.0.0", "1.5.0", "2.0.0", "latest")
	spec := &config.ImageSpec{
		Image: host + "/apps/sentinel",
		Tags: config.TagStrategy{
			Strategy: "pattern",
			Pattern:  `^\d+\.\d+\.\d+$`,
			MaxTags:  2,
		},
	}

	// When
	got, err := FindTagsToPatch(spec)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []string{"1.5.0", "2.0.0"}, got)
}

func Test_FindTagsToPatch_returns_lexical_tail_when_pattern_tags_are_not_semver(t *testing.T) {
	// Given
	host := newTagBoundaryRegistry(t)
	pushTagBoundaryImages(t, host, "apps/sentinel", "build-202601", "build-202603", "build-202602")
	spec := &config.ImageSpec{
		Image: host + "/apps/sentinel",
		Tags: config.TagStrategy{
			Strategy: "pattern",
			Pattern:  `^build-`,
			MaxTags:  2,
		},
	}

	// When
	got, err := FindTagsToPatch(spec)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []string{"build-202602", "build-202603"}, got)
}

func Test_FindTagsToPatch_returns_empty_when_latest_has_no_semver_candidates(t *testing.T) {
	// Given
	host := newTagBoundaryRegistry(t)
	pushTagBoundaryImages(t, host, "apps/sentinel", "edge", "latest")
	spec := &config.ImageSpec{
		Image: host + "/apps/sentinel",
		Tags:  config.TagStrategy{Strategy: "latest"},
	}

	// When
	got, err := FindTagsToPatch(spec)

	// Then
	require.NoError(t, err)
	assert.Empty(t, got)
}

func Test_FindTagsToPatch_returns_highest_semver_for_latest_strategy(t *testing.T) {
	// Given
	host := newTagBoundaryRegistry(t)
	pushTagBoundaryImages(t, host, "apps/sentinel", "1.0.0", "2.0.0", "latest")
	spec := &config.ImageSpec{
		Image: host + "/apps/sentinel",
		Tags:  config.TagStrategy{Strategy: "latest"},
	}

	// When
	got, err := FindTagsToPatch(spec)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []string{"2.0.0"}, got)
}

func Test_FindTagsToPatch_returns_error_when_pattern_is_invalid(t *testing.T) {
	// Given
	host := newTagBoundaryRegistry(t)
	pushTagBoundaryImages(t, host, "apps/sentinel", "1.0.0")
	spec := &config.ImageSpec{
		Image: host + "/apps/sentinel",
		Tags: config.TagStrategy{
			Strategy: "pattern",
			Pattern:  "[",
		},
	}

	// When
	_, err := FindTagsToPatch(spec)

	// Then
	require.Error(t, err)
}

func Test_FindTagsToPatch_propagates_registry_errors_at_strategy_boundaries(t *testing.T) {
	tests := []string{"latest", "pattern"}
	for _, strategy := range tests {
		t.Run(strategy, func(t *testing.T) {
			// Given
			server := httptest.NewServer(registry.New())
			host := strings.TrimPrefix(server.URL, "http://")
			server.Close()
			spec := &config.ImageSpec{
				Image: host + "/apps/sentinel",
				Tags: config.TagStrategy{
					Strategy: strategy,
					Pattern:  `.*`,
				},
			}

			// When
			_, err := FindTagsToPatch(spec)

			// Then
			require.Error(t, err)
		})
	}
}

func Test_selectPatternTags_honors_exclusion_and_limit_boundaries(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		exclusions []string
		maxTags    int
		want       []string
	}{
		{
			name: "zero limit keeps every semantic version",
			tags: []string{"2.0.0", "1.0.0"},
			want: []string{"1.0.0", "2.0.0"},
		},
		{
			name:       "all matches excluded returns an empty slice",
			tags:       []string{"edge", "latest"},
			exclusions: []string{"edge", "latest"},
			maxTags:    1,
			want:       []string{},
		},
		{
			name:    "semantic versions take precedence over non-semver matches",
			tags:    []string{"latest", "1.0.0", "2.0.0"},
			maxTags: 1,
			want:    []string{"2.0.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := selectPatternTags(tt.tags, tt.exclusions, tt.maxTags)

			// Then
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_craneOptions_recognizes_ipv6_loopback_with_port(t *testing.T) {
	// Given
	image := "[::1]:5000/apps/sentinel:1.0.0"

	// When
	got := craneOptions(image)

	// Then
	assert.Len(t, got, 1)
}

func newTagBoundaryRegistry(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "http://")
}

func pushTagBoundaryImages(t *testing.T, host, repository string, tags ...string) {
	t.Helper()
	for _, tag := range tags {
		ref := fmt.Sprintf("%s/%s:%s", host, repository, tag)
		require.NoError(t, crane.Push(empty.Image, ref, crane.Insecure))
	}
}
