import { REGISTRY } from "../data/full-catalog";

const REGISTRY_PREFIX = REGISTRY + "/";

/**
 * Extract the catalog name from a patched image reference by stripping
 * the registry prefix, digest, and tag.
 *
 * e.g. "ghcr.io/verity-org/kiwigrid/k8s-sidecar:1.28.0" → "kiwigrid/k8s-sidecar"
 */
export function patchedRefToName(ref: string | undefined): string {
  if (!ref) return "";

  let v = ref;
  const at = v.indexOf("@");
  if (at !== -1) v = v.slice(0, at);

  const lastSlash = v.lastIndexOf("/");
  const lastColon = v.lastIndexOf(":");
  if (lastColon > lastSlash) v = v.slice(0, lastColon);

  if (v.startsWith(REGISTRY_PREFIX)) {
    return v.slice(REGISTRY_PREFIX.length);
  }

  const parts = v.split("/");
  if (parts.length >= 2) {
    const first = parts[0] ?? "";
    if (/[.:]/.test(first) || first === "localhost") {
      return parts.slice(1).join("/");
    }
  }

  return v;
}

/**
 * Extract the upstream version from an image reference by taking the tag
 * and stripping the `-patched` suffix. Digest-pinned refs (`@sha256:...`)
 * are stripped first so the digest is never mistaken for a tag.
 *
 * e.g. "docker.io/rabbitmqoperator/cluster-operator:2.19.1" → "2.19.1"
 *      "ghcr.io/verity-org/nginx:1.29.3-patched"            → "1.29.3"
 *      "gcr.io/distroless/static@sha256:abc123"             → ""
 */
export function extractVersionFromRef(ref: string): string {
  let v = ref;
  const at = v.indexOf("@");
  if (at !== -1) v = v.slice(0, at);
  const lastColon = v.lastIndexOf(":");
  if (lastColon < 0) return "";
  const tag = v.slice(lastColon + 1);
  return tag.replace(/-patched$/, "");
}
