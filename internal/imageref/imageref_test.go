package imageref

import "testing"

func TestRepoPath(t *testing.T) {
	cases := []struct {
		ref  string
		want string
	}{
		{"quay.io/prometheus/prometheus:v3.11.2", "prometheus/prometheus"},
		{"ghcr.io/dexidp/dex:v2.45.1", "dexidp/dex"},
		{"reg.kyverno.io/kyverno/kyverno-cli:v1.17.2", "kyverno/kyverno-cli"},
		{"opensearchproject/opensearch:3.6.0", "opensearchproject/opensearch"},
		{"nats:2.12.6-alpine", "nats"},
		{"docker.io/library/nginx:1.29.5", "library/nginx"},
		{"quay.io/cilium/cilium:v1.19.3@sha256:abc", "cilium/cilium"},
		{"localhost:5000/foo/bar:1", "foo/bar"},
		{"plain", "plain"},
	}
	for _, c := range cases {
		if got := RepoPath(c.ref); got != c.want {
			t.Errorf("RepoPath(%q) = %q, want %q", c.ref, got, c.want)
		}
	}
}

func TestSplitRef(t *testing.T) {
	cases := []struct {
		ref      string
		wantRepo string
		wantTag  string
	}{
		{"quay.io/prometheus/prometheus:v3.11.2", "quay.io/prometheus/prometheus", "v3.11.2"},
		{"foo/bar", "foo/bar", ""},
		{"foo/bar:tag@sha256:abc", "foo/bar", "tag"},
	}
	for _, c := range cases {
		gotRepo, gotTag := SplitRef(c.ref)
		if gotRepo != c.wantRepo || gotTag != c.wantTag {
			t.Errorf("SplitRef(%q) = (%q, %q), want (%q, %q)", c.ref, gotRepo, gotTag, c.wantRepo, c.wantTag)
		}
	}
}
