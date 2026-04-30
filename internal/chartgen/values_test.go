package chartgen

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
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
				Path:          "image",
				Repository:    "ghcr.io/verity-org/prometheus/prometheus",
				Tag:           "v3.2.1",
				ClearRegistry: false,
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
				Path:          "server.image",
				Repository:    "ghcr.io/verity-org/library/nginx",
				Tag:           "1.25",
				ClearRegistry: false,
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
					Path:          "controller.image",
					Repository:    "ghcr.io/verity-org/library/nginx",
					Tag:           "1.25",
					ClearRegistry: false,
				},
				{
					Path:          "metrics.image",
					Repository:    "ghcr.io/verity-org/valkey/valkey",
					Tag:           "7.2",
					ClearRegistry: false,
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
				Path:          "custom.image",
				Repository:    "ghcr.io/verity-org/library/nginx",
				Tag:           "1.25",
				ClearRegistry: false,
			}},
		},
		{
			name: "explicit value path clears registry on 3-field shape",
			valuesYML: `image:
  registry: ghcr.io
  repository: zalando/postgres-operator
  tag: v1.15.1
`,
			mappings: []ImageMapping{{
				OriginalRepo: "zalando/postgres-operator",
				PatchedRepo:  "ghcr.io/verity-org/zalando/postgres-operator",
				PatchedTag:   "v1.15.1",
			}},
			overrides: map[string]config.Override{
				"zalando/postgres-operator": {ValuePath: "image"},
			},
			want: []ValueOverride{{
				Path:          "image",
				Repository:    "ghcr.io/verity-org/zalando/postgres-operator",
				Tag:           "v1.15.1",
				ClearRegistry: true,
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
				Path:          "custom.vectorImage",
				Repository:    "ghcr.io/verity-org/timberio/vector",
				Tag:           "0.40",
				ClearRegistry: false,
			}},
		},
		{
			name: "3-field registry split override",
			valuesYML: `image:
  registry: ghcr.io
  repository: zalando/postgres-operator
  tag: v1.15.1
`,
			mappings: []ImageMapping{{
				OriginalRepo: "zalando/postgres-operator",
				PatchedRepo:  "ghcr.io/verity-org/zalando/postgres-operator",
				PatchedTag:   "v1.15.1",
			}},
			want: []ValueOverride{{
				Path:          "image",
				Repository:    "ghcr.io/verity-org/zalando/postgres-operator",
				Tag:           "v1.15.1",
				ClearRegistry: true,
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

func TestExtractValuesFromTarball(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantName    string
		wantValues  string
		wantErr     bool
		wantEmpty   bool
	}{
		{
			name: "valid tgz with values.yaml",
			setup: func(t *testing.T) string {
				return writeTestChartTarball(t, "myalert", map[string]string{
					"Chart.yaml":  "name: myalert\nversion: 0.1.0\n",
					"values.yaml": "image:\n  repository: quay.io/foo\n  tag: \"1.0\"\n",
				})
			},
			wantName:   "myalert",
			wantValues: "repository: quay.io/foo",
		},
		{
			name: "tgz without values.yaml",
			setup: func(t *testing.T) string {
				return writeTestChartTarball(t, "foo", map[string]string{
					"Chart.yaml": "name: foo\nversion: 0.1.0\n",
				})
			},
			wantName:  "foo",
			wantEmpty: true,
		},
		{
			name: "non-existent file",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.tgz")
			},
			wantErr: true,
		},
		{
			name: "corrupt tarball",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "corrupt.tgz")
				if err := os.WriteFile(path, []byte("not a tarball"), 0o644); err != nil {
					t.Fatalf("os.WriteFile() error = %v", err)
				}
				return path
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)

			gotValues, gotName, err := extractValuesFromTarball(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("extractValuesFromTarball() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("extractValuesFromTarball() error = %v", err)
			}
			if gotName != tt.wantName {
				t.Fatalf("extractValuesFromTarball() chartName = %q, want %q", gotName, tt.wantName)
			}
			if tt.wantEmpty {
				if len(gotValues) != 0 {
					t.Fatalf("extractValuesFromTarball() values = %q, want empty", string(gotValues))
				}
				return
			}
			if !strings.Contains(string(gotValues), tt.wantValues) {
				t.Fatalf("extractValuesFromTarball() values = %q, want substring %q", string(gotValues), tt.wantValues)
			}
		})
	}
}

func TestEnumerateSubchartArchives(t *testing.T) {
	chartsDir := t.TempDir()
	alpha := writeTestChartTarballInDir(t, chartsDir, "alpha", map[string]string{
		"Chart.yaml": "name: alpha\nversion: 0.1.0\n",
	})
	beta := writeTestChartTarballInDir(t, chartsDir, "beta", map[string]string{
		"Chart.yaml": "name: beta\nversion: 0.2.0\n",
	})
	if err := os.WriteFile(filepath.Join(chartsDir, "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(chartsDir, "gamma"), 0o755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}

	got, err := enumerateSubchartArchives(chartsDir)
	if err != nil {
		t.Fatalf("enumerateSubchartArchives() error = %v", err)
	}

	want := []string{alpha, beta}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("enumerateSubchartArchives() = %#v, want %#v", got, want)
	}
}

func TestResolveValuePathsWithSubcharts(t *testing.T) {
	tests := []struct {
		name           string
		parentValues   string
		subchartValues map[string]string
		mappings       []ImageMapping
		overrides      map[string]config.Override
		want           []ValueOverride
	}{
		{
			name:         "prometheus subchart override",
			parentValues: "server:\n  enabled: true\n",
			subchartValues: map[string]string{
				"alertmanager": `image:
  repository: quay.io/prometheus/alertmanager
  tag: v0.28.1
`,
				"kube-state-metrics": `image:
  repository: registry.k8s.io/kube-state-metrics/kube-state-metrics
  tag: v2.15.0
`,
			},
			mappings: []ImageMapping{
				{
					OriginalRepo: "quay.io/prometheus/alertmanager",
					PatchedRepo:  "ghcr.io/verity-org/prometheus/alertmanager",
					PatchedTag:   "v0.28.1",
				},
				{
					OriginalRepo: "registry.k8s.io/kube-state-metrics/kube-state-metrics",
					PatchedRepo:  "ghcr.io/verity-org/kube-state-metrics/kube-state-metrics",
					PatchedTag:   "v2.15.0",
				},
			},
			want: []ValueOverride{
				{
					Path:       "alertmanager.image",
					Repository: "ghcr.io/verity-org/prometheus/alertmanager",
					Tag:        "v0.28.1",
				},
				{
					Path:       "kube-state-metrics.image",
					Repository: "ghcr.io/verity-org/kube-state-metrics/kube-state-metrics",
					Tag:        "v2.15.0",
				},
			},
		},
		{
			name: "parent + subchart combined",
			parentValues: `image:
  repository: nginx
  tag: "1.25"
`,
			subchartValues: map[string]string{
				"metrics": `image:
  repository: valkey
  tag: "7.2"
`,
			},
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
					Path:       "image",
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
			name:         "subchart without matching mapping",
			parentValues: "replicaCount: 1\n",
			subchartValues: map[string]string{
				"pushgateway": `image:
  repository: quay.io/prometheus/pushgateway
  tag: v1.11.1
`,
			},
			mappings: []ImageMapping{ {
				OriginalRepo: "nginx",
				PatchedRepo:  "ghcr.io/verity-org/library/nginx",
				PatchedTag:   "1.25",
			}},
			want: []ValueOverride{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subcharts := make(map[string][]byte, len(tt.subchartValues))
			for name, values := range tt.subchartValues {
				subcharts[name] = []byte(values)
			}

			got, err := ResolveValuePathsWithSubcharts([]byte(tt.parentValues), subcharts, tt.mappings, tt.overrides)
			if err != nil {
				t.Fatalf("ResolveValuePathsWithSubcharts() error = %v", err)
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
				t.Fatalf("ResolveValuePathsWithSubcharts() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestWalkValues(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		yamlIn string
		want   []repoTagPair
	}{
		{
			name:   "with prefix",
			prefix: "foo",
			yamlIn: `image:
  repository: nginx
  tag: "1"
`,
			want: []repoTagPair{{Path: "foo.image", Repo: "nginx", HasTag: true}},
		},
		{
			name: "flat",
			yamlIn: `image:
  repository: nginx
  tag: "1.25"
`,
			want: []repoTagPair{{Path: "image", Repo: "nginx", HasTag: true, Registry: "", HasRegistry: false}},
		},
		{
			name: "deep",
			yamlIn: `a:
  b:
    image:
      repository: r
      tag: t
`,
			want: []repoTagPair{{Path: "a.b.image", Repo: "r", HasTag: true, Registry: "", HasRegistry: false}},
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
				{Path: "image", Repo: "nginx", HasTag: true, Registry: "", HasRegistry: false},
				{Path: "server.image", Repo: "valkey", HasTag: true, Registry: "", HasRegistry: false},
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
			want: []repoTagPair{{Path: "image", Repo: "nginx", HasTag: false, Registry: "", HasRegistry: false}},
		},
		{
			name: "registry-repository-tag triple",
			yamlIn: `image:
  registry: "ghcr.io"
  repository: "zalando/postgres-operator"
  tag: "v1.15.1"
`,
			want: []repoTagPair{{
				Path:        "image",
				Repo:        "zalando/postgres-operator",
				HasTag:      true,
				Registry:    "ghcr.io",
				HasRegistry: true,
			}},
		},
		{
			name: "registry without repository",
			yamlIn: `image:
  registry: "ghcr.io"
  tag: "v1"
`,
			want: []repoTagPair{},
		},
		{
			name: "registry non-string ignored",
			yamlIn: `image:
  registry: 123
  repository: "foo"
  tag: "v1"
`,
			want: []repoTagPair{{
				Path:        "image",
				Repo:        "foo",
				HasTag:      true,
				Registry:    "",
				HasRegistry: false,
			}},
		},
		{
			name: "registry empty string ignored",
			yamlIn: `image:
  registry: ""
  repository: "foo"
  tag: "v1"
`,
			want: []repoTagPair{{
				Path:        "image",
				Repo:        "foo",
				HasTag:      true,
				Registry:    "",
				HasRegistry: false,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make(map[string]any)
			if err := yaml.Unmarshal([]byte(tt.yamlIn), &data); err != nil {
				t.Fatalf("yaml.Unmarshal() error = %v", err)
			}

			got := make([]repoTagPair, 0)
			walkValues(tt.prefix, data, &got)

			sort.Slice(got, func(i, j int) bool {
				if got[i].Path != got[j].Path {
					return got[i].Path < got[j].Path
				}
				if got[i].Repo != got[j].Repo {
					return got[i].Repo < got[j].Repo
				}
				if got[i].Registry != got[j].Registry {
					return got[i].Registry < got[j].Registry
				}
				if got[i].HasRegistry != got[j].HasRegistry {
					return !got[i].HasRegistry && got[j].HasRegistry
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
				if tt.want[i].Registry != tt.want[j].Registry {
					return tt.want[i].Registry < tt.want[j].Registry
				}
				if tt.want[i].HasRegistry != tt.want[j].HasRegistry {
					return !tt.want[i].HasRegistry && tt.want[j].HasRegistry
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

func writeTestChartTarball(t *testing.T, chartName string, files map[string]string) string {
	t.Helper()

	return writeTestChartTarballInDir(t, t.TempDir(), chartName, files)
}

func writeTestChartTarballInDir(t *testing.T, dir, chartName string, files map[string]string) string {
	t.Helper()

	path := filepath.Join(dir, chartName+".tgz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}

	gz := gzip.NewWriter(file)

	tw := tar.NewWriter(gz)

	for name, content := range files {
		data := []byte(content)
		header := &tar.Header{
			Name: filepath.ToSlash(filepath.Join(chartName, name)),
			Mode: 0o644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("tw.WriteHeader() error = %v", err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("tw.Write() error = %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close() error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz.Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file.Close() error = %v", err)
	}

	return path
}
