// Package imageref parses container image references for Verity tooling:
// strips registry hosts, tags, and digests so callers can match image refs
// by repository path. Shared by internal/chartgen and internal/doctor to
// ensure they reason about images identically — a divergence between the
// chart-gen runtime matcher and the doctor lint would silently hide the
// exact silent failures doctor exists to surface.
package imageref

import "strings"

// RepoPath returns the registry-stripped, tag/digest-stripped portion of a
// container image reference. e.g.,
//
//	ghcr.io/dexidp/dex:v2.45.1            → dexidp/dex
//	quay.io/cilium/cilium:v1.19.3@sha256:… → cilium/cilium
//	docker.io/library/nginx:1.29.5        → library/nginx
//	nats:2.12.6-alpine                    → nats
//
// A "registry" is detected as the first path segment that contains a dot
// or colon, or that is literally "localhost".
func RepoPath(ref string) string {
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}
	lastSlash := strings.LastIndex(ref, "/")
	if lastColon := strings.LastIndex(ref, ":"); lastColon > lastSlash {
		ref = ref[:lastColon]
	}
	parts := strings.Split(ref, "/")
	if len(parts) >= 2 {
		first := parts[0]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			return strings.Join(parts[1:], "/")
		}
	}
	return ref
}

// SplitRef splits an image reference into (repo, tag). Digests are stripped
// before splitting.
func SplitRef(ref string) (repo, tag string) {
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}
	lastSlash := strings.LastIndex(ref, "/")
	if lastColon := strings.LastIndex(ref, ":"); lastColon > lastSlash {
		return ref[:lastColon], ref[lastColon+1:]
	}
	return ref, ""
}
