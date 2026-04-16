package chartgen

import (
	"reflect"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
)

func TestResolveValuePaths(t *testing.T) {
	tests := []struct {
		name      string
		valuesYML string
		mappings  []ImageMapping
		overrides map[string]config.Override
		want      []ValueOverride
	}{
		{
			name: "simple flat values",
			valuesYML: `image:
  repository: quay.io/prometheus/prometheus
  tag: v3.2.1
`,
			mappings: []ImageMapping{
				{
					OriginalRepo: "quay.io/prometheus/prometheus",
					PatchedRepo:  "ghcr.io/verity-org/prometheus/prometheus",
					PatchedTag:   "v3.2.1",
				},
			},
			want: []ValueOverride{{
				Path:       "image",
				Repository: "ghcr.io/verity-org/prometheus/prometheus",
				Tag:        "v3.2.1",
			}},
		},
		{
			name: "nested image values",
			valuesYML: `server:
  image:
    repository: nginx
    tag: "1.25"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "nginx",
				PatchedRepo:  "ghcr.io/verity-org/library/nginx",
				PatchedTag:   "1.25",
			}},
			want: []ValueOverride{{
				Path:       "server.image",
				Repository: "ghcr.io/verity-org/library/nginx",
				Tag:        "1.25",
			}},
		},
		{
			name: "no matching mapping",
			valuesYML: `image:
  repository: nginx
  tag: "1.25"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "valkey",
				PatchedRepo:  "ghcr.io/verity-org/valkey/valkey",
				PatchedTag:   "7.0",
			}},
			want: []ValueOverride{},
		},
		{
			name: "multiple images",
			valuesYML: `controller:
  image:
    repository: nginx
    tag: "1.25"
metrics:
  image:
    repository: valkey
    tag: "7.2"
`,
			mappings: []ImageMapping{
				{
					OriginalRepo: "nginx",
					PatchedRepo:  "ghcr.io/verity-org/library/nginx",
					PatchedTag:   "1.25",
				},
				{
					OriginalRepo: "valkey",
					PatchedRepo:  "ghcr.io/verity-org/valkey/valkey",
					PatchedTag:   "7.2",
				},
			},
			want: []ValueOverride{
				{
					Path:       "controller.image",
					Repository: "ghcr.io/verity-org/library/nginx",
					Tag:        "1.25",
				},
				{
					Path:       "metrics.image",
					Repository: "ghcr.io/verity-org/valkey/valkey",
					Tag:        "7.2",
				},
			},
		},
		{
			name: "explicit value path takes precedence",
			valuesYML: `image:
  repository: nginx
  tag: "1.25"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "nginx",
				PatchedRepo:  "ghcr.io/verity-org/library/nginx",
				PatchedTag:   "1.25",
			}},
			overrides: map[string]config.Override{
				"nginx": {ValuePath: "custom.image"},
			},
			want: []ValueOverride{{
				Path:       "custom.image",
				Repository: "ghcr.io/verity-org/library/nginx",
				Tag:        "1.25",
			}},
		},
		{
			name: "override key suffix matching",
			valuesYML: `vector:
  repository: docker.io/timberio/vector
  tag: "0.40"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "docker.io/timberio/vector",
				PatchedRepo:  "ghcr.io/verity-org/timberio/vector",
				PatchedTag:   "0.40",
			}},
			overrides: map[string]config.Override{
				"timberio/vector": {ValuePath: "custom.vectorImage"},
			},
			want: []ValueOverride{{
				Path:       "custom.vectorImage",
				Repository: "ghcr.io/verity-org/timberio/vector",
				Tag:        "0.40",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveValuePaths([]byte(tt.valuesYML), tt.mappings, tt.overrides)
			if err != nil {
				t.Fatalf("ResolveValuePaths() error = %v", err)
			}

			sort.Slice(got, func(i, j int) bool {
				if got[i].Path != got[j].Path {
					return got[i].Path < got[j].Path
				}
				if got[i].Repository != got[j].Repository {
					return got[i].Repository < got[j].Repository
				}
				return got[i].Tag < got[j].Tag
			})
			sort.Slice(tt.want, func(i, j int) bool {
				if tt.want[i].Path != tt.want[j].Path {
					return tt.want[i].Path < tt.want[j].Path
				}
				if tt.want[i].Repository != tt.want[j].Repository {
					return tt.want[i].Repository < tt.want[j].Repository
				}
				return tt.want[i].Tag < tt.want[j].Tag
			})

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResolveValuePaths() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestWalkValues(t *testing.T) {
	tests := []struct {
		name   string
		yamlIn string
		want   []repoTagPair
	}{
		{
			name: "flat",
			yamlIn: `image:
  repository: nginx
  tag: "1.25"
`,
			want: []repoTagPair{{Path: "image", Repo: "nginx", HasTag: true}},
		},
		{
			name: "deep",
			yamlIn: `a:
  b:
    image:
      repository: r
      tag: t
`,
			want: []repoTagPair{{Path: "a.b.image", Repo: "r", HasTag: true}},
		},
		{
			name: "multiple pairs",
			yamlIn: `image:
  repository: nginx
  tag: "1.25"
server:
  image:
    repository: valkey
    tag: "7.2"
`,
			want: []repoTagPair{
				{Path: "image", Repo: "nginx", HasTag: true},
				{Path: "server.image", Repo: "valkey", HasTag: true},
			},
		},
		{
			name: "no repository sibling",
			yamlIn: `image:
  repo: nginx
  tag: "1.25"
`,
			want: []repoTagPair{},
		},
		{
			name: "repository without tag",
			yamlIn: `image:
  repository: nginx
`,
			want: []repoTagPair{{Path: "image", Repo: "nginx", HasTag: false}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make(map[string]any)
			if err := yaml.Unmarshal([]byte(tt.yamlIn), &data); err != nil {
				t.Fatalf("yaml.Unmarshal() error = %v", err)
			}

			got := make([]repoTagPair, 0)
			walkValues("", data, &got)

			sort.Slice(got, func(i, j int) bool {
				if got[i].Path != got[j].Path {
					return got[i].Path < got[j].Path
				}
				if got[i].Repo != got[j].Repo {
					return got[i].Repo < got[j].Repo
				}
				if got[i].HasTag == got[j].HasTag {
					return false
				}
				return !got[i].HasTag && got[j].HasTag
			})
			sort.Slice(tt.want, func(i, j int) bool {
				if tt.want[i].Path != tt.want[j].Path {
					return tt.want[i].Path < tt.want[j].Path
				}
				if tt.want[i].Repo != tt.want[j].Repo {
					return tt.want[i].Repo < tt.want[j].Repo
				}
				if tt.want[i].HasTag == tt.want[j].HasTag {
					return false
				}
				return !tt.want[i].HasTag && tt.want[j].HasTag
			})

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("walkValues() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMatchesRepo(t *testing.T) {
	tests := []struct {
		name      string
		imageRepo string
		candidate string
		want      bool
	}{
		{name: "exact", imageRepo: "nginx", candidate: "nginx", want: true},
		{name: "suffix", imageRepo: "docker.io/library/nginx", candidate: "nginx", want: true},
		{name: "reverse suffix", imageRepo: "nginx", candidate: "docker.io/library/nginx", want: true},
		{name: "no match", imageRepo: "valkey", candidate: "nginx", want: false},
		{name: "partial non suffix", imageRepo: "inx", candidate: "nginx", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesRepo(tt.imageRepo, tt.candidate)
			if got != tt.want {
				t.Fatalf("matchesRepo(%q, %q) = %v, want %v", tt.imageRepo, tt.candidate, got, tt.want)
			}
		})
	}
}
