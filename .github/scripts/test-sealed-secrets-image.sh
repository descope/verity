#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <image-ref> <version> <full-version>" >&2
  exit 2
fi

image_ref=$1
version=$2
full_version=$3
container=''
sbom="${RUNNER_TEMP:-/tmp}/sealed-secrets-${full_version}.spdx.json"

cleanup() {
  if [[ -n "$container" ]]; then
    docker rm --force "$container" >/dev/null 2>&1 || true
  fi
  rm -f "$sbom"
}
trap cleanup EXIT

docker run --rm --entrypoint /usr/bin/controller "$image_ref" --version \
  | grep -Fx "controller version: v${version}"
docker run --rm --entrypoint /usr/bin/controller "$image_ref" --help >/dev/null
docker run --rm --entrypoint /usr/bin/kubeseal "$image_ref" --help >/dev/null

container=$(docker create "$image_ref")
# ca-certificates-bundle must be present at runtime even though the base also carries it.
docker export "$container" \
  | tar -tf - \
  | grep -E '(^|/)etc/ssl/(cert.pem|certs/ca-certificates.crt)$'
docker cp \
  "$container:/var/lib/db/sbom/sealed-secrets-0-${full_version}.spdx.json" \
  "$sbom"
jq -e --arg full_version "$full_version" '
  .spdxVersion == "SPDX-2.3"
  and any(
    .packages[];
    .name == "sealed-secrets-0"
    and .versionInfo == $full_version
    and .licenseDeclared == "Apache-2.0"
  )
' "$sbom"
