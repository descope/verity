#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <image> <type> [version]" >&2
  echo "  image  image name, e.g. caddy" >&2
  echo "  type   image type, e.g. fips" >&2
  echo "  version optional image version stream, e.g. 14" >&2
  echo "" >&2
  echo "Runs the melange prep + build steps locally, mirroring CI." >&2
  echo "Requires: jq, yq, git, melange (install via: mise install)" >&2
  exit 1
}

[[ $# -eq 2 || $# -eq 3 ]] || usage

IMAGE="$1"
TYPE="$2"
VERSION="${3:-}"

validate_ci_identifier() {
  local label="$1" value="$2"
  if [[ ! "$value" =~ ^[a-z][a-z0-9-]*$ ]]; then
    echo "Invalid ${label}: '${value}'" >&2
    echo "${label} must match ^[a-z][a-z0-9-]*$ (lowercase letters, digits, hyphens)" >&2
    exit 1
  fi
}

validate_ci_identifier "image" "$IMAGE"
validate_ci_identifier "type"  "$TYPE"

tmp_output=$(mktemp)
trap 'rm -f "$tmp_output"' EXIT

GITHUB_OUTPUT="$tmp_output" IMAGE="$IMAGE" TYPE="$TYPE" VERSION="$VERSION" bash .github/scripts/melange-check.sh

needed=$(grep '^needed=' "$tmp_output" | cut -d= -f2-)
if [ "$needed" != "true" ]; then
  echo "No melange block for ${IMAGE}:${TYPE}${VERSION:+:${VERSION}} — nothing to do"
  exit 0
fi

specs_json=$(grep '^specs_json=' "$tmp_output" | cut -d= -f2-)
printf '%s' "$specs_json" | jq -c '.[]' | while IFS= read -r spec; do
  BESPOKE=$(printf '%s' "$spec" | jq -r '.bespoke // ""')
  UPSTREAM=$(printf '%s' "$spec" | jq -r '.upstream // ""')
  ENV_FILE=$(printf '%s' "$spec" | jq -r '."env-file" // ""')
  BUILD_OPTION=$(printf '%s' "$spec" | jq -r '."build-option" // ""')
  export BESPOKE UPSTREAM ENV_FILE BUILD_OPTION
  bash .github/scripts/melange-build.sh
done

echo ""
echo "Built packages:"
find packages/repo -type f -name '*.apk'
