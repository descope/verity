#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <image> <type>" >&2
  echo "  image  image name, e.g. caddy" >&2
  echo "  type   image type, e.g. fips" >&2
  echo "" >&2
  echo "Runs the melange prep + build steps locally, mirroring CI." >&2
  echo "Requires: jq, yq, sha256sum (or shasum), awk, melange (install via: mise install)" >&2
  exit 1
}

[[ $# -eq 2 ]] || usage

IMAGE="$1"
TYPE="$2"

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


resolve_version_template() {
  local value="$1"
  if [[ "$value" == *"{{version}}"* ]]; then
    if [ -z "${VERSION:-}" ]; then
      echo "melange value ${value} contains {{version}} but VERSION is not set" >&2
      exit 1
    fi
    value=${value//\{\{version\}\}/$VERSION}
  fi
  printf '%s' "$value"
}

image_yaml="images/${IMAGE}.yaml"
if [ ! -f "$image_yaml" ]; then
  echo "Image config not found: ${image_yaml}" >&2
  exit 1
fi

melange_block=$(yq -e ".types.\"${TYPE}\".melange" "$image_yaml" 2>/dev/null) || true
if [ -z "$melange_block" ] || [ "$melange_block" = "null" ]; then
  echo "No melange block for ${IMAGE}:${TYPE} — nothing to do"
  exit 0
fi

UPSTREAM=$(resolve_version_template "$(yq -r ".types.\"${TYPE}\".melange.upstream // \"\"" "$image_yaml")")
BESPOKE=$(resolve_version_template "$(yq -r ".types.\"${TYPE}\".melange.bespoke // \"\"" "$image_yaml")")
ENV_FILE=$(resolve_version_template "$(yq -r ".types.\"${TYPE}\".melange.env-file // \"\"" "$image_yaml")")
BUILD_OPTION=$(resolve_version_template "$(yq -r ".types.\"$TYPE\".melange.build-option // \"\"" "$image_yaml")")

validate_filename() {
  local label="$1" value="$2"
  if [[ ! "$value" =~ ^[A-Za-z0-9._-]+$ ]] || [[ "$value" == *".."* ]]; then
    echo "$label contains unsafe characters: '$value'" >&2
    exit 1
  fi
}

[ -n "$BESPOKE" ] && validate_filename "bespoke" "$BESPOKE"
[ -n "$ENV_FILE" ] && validate_filename "env-file" "$ENV_FILE"
[ -n "$BUILD_OPTION" ] && validate_filename "build-option" "$BUILD_OPTION"

case "$(uname -m)" in
  x86_64) BUILD_ARCH="x86_64" ;;
  aarch64|arm64) BUILD_ARCH="aarch64" ;;
  *) echo "Unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac

BESPOKE_JSON=$(jq -cn --arg recipe "$BESPOKE" 'if $recipe == "" then [] else [$recipe] end')
rm -rf melange-work packages/repo
export UPSTREAM BESPOKE_JSON ENV_FILE BUILD_OPTION BUILD_ARCH

.github/scripts/melange-build.sh

echo
echo "Built packages:"
find packages/repo -type f -name '*.apk'
