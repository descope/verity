#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <image> <type>" >&2
  echo "  image  image name, e.g. caddy" >&2
  echo "  type   image type, e.g. fips" >&2
  echo "" >&2
  echo "Runs the melange prep + build steps locally, mirroring CI." >&2
  echo "Requires: jq, yq, curl, git, sha256sum (or shasum), awk, melange (install via: mise install)" >&2
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

UPSTREAM=$(yq -r ".types.\"${TYPE}\".melange.upstream // \"\"" "$image_yaml")
BESPOKE=$(yq -r ".types.\"${TYPE}\".melange.bespoke // \"\"" "$image_yaml")
ENV_FILE=$(yq -r ".types.\"${TYPE}\".melange.env-file // \"\"" "$image_yaml")
BUILD_OPTION=$(yq -r ".types.\"${TYPE}\".melange.build-option // \"\"" "$image_yaml")

# Validate a filename value: must be non-empty, contain only safe characters,
# and must not contain path separators or traversal sequences.
validate_filename() {
  local label="$1" value="$2"
  if [[ ! "$value" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "${label} contains invalid characters: '${value}'" >&2
    echo "Only alphanumeric characters, dots, underscores, and hyphens are allowed." >&2
    exit 1
  fi
  if [[ "$value" == *".."* ]]; then
    echo "${label} must not contain path traversal sequences ('..'): '${value}'" >&2
    exit 1
  fi
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

[ -n "$BESPOKE" ]  && validate_filename "bespoke"  "$BESPOKE"
[ -n "$ENV_FILE" ] && validate_filename "env-file"  "$ENV_FILE"

rm -rf melange-work
mkdir -p melange-work

if [ -n "$BESPOKE" ]; then
  cp "packages/bespoke/${BESPOKE}" melange-work/build.yaml
elif [ -n "$UPSTREAM" ]; then
  commit=$(jq -r '.wolfi_commit' packages/upstream.lock.json)
  if [ "$commit" = "null" ] || [ -z "$commit" ]; then
    echo "wolfi_commit missing or null in packages/upstream.lock.json" >&2
    exit 1
  fi
  file=$(jq -r --arg pkg "$UPSTREAM" '.packages[$pkg].file' packages/upstream.lock.json)
  expected_sha=$(jq -r --arg pkg "$UPSTREAM" '.packages[$pkg].sha256' packages/upstream.lock.json)
  if [ "$file" = "null" ] || [ -z "$file" ]; then
    echo "Package '${UPSTREAM}' not found in upstream.lock.json" >&2
    exit 1
  fi
  if [ "$expected_sha" = "null" ] || [ -z "$expected_sha" ]; then
    echo "No sha256 for '${UPSTREAM}' in upstream.lock.json" >&2
    exit 1
  fi

  url="https://raw.githubusercontent.com/wolfi-dev/os/${commit}/${file}"
  echo "Fetching upstream melange YAML: ${url}"
  curl -fsSL "$url" -o melange-work/build.yaml.tmp
  actual_sha=$(sha256_file melange-work/build.yaml.tmp)
  if [ "$actual_sha" != "$expected_sha" ]; then
    echo "sha256 mismatch for ${UPSTREAM}: expected ${expected_sha}, got ${actual_sha}" >&2
    rm -f melange-work/build.yaml.tmp
    exit 1
  fi
  mv melange-work/build.yaml.tmp melange-work/build.yaml

  if [[ ! "$UPSTREAM" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "upstream value contains unsafe characters: '${UPSTREAM}'" >&2
    exit 1
  fi

  echo "Fetching wolfi pipelines/ and ${UPSTREAM}/ companion dir at commit ${commit}"
  tmp_wolfi=$(mktemp -d)
  trap 'rm -rf "$tmp_wolfi"' EXIT
  git -C "$tmp_wolfi" init --quiet
  git -C "$tmp_wolfi" remote add origin "https://github.com/wolfi-dev/os.git"
  git -C "$tmp_wolfi" sparse-checkout set --no-cone pipelines "${UPSTREAM}"
  git -C "$tmp_wolfi" fetch --quiet --depth 1 --filter=blob:none origin "$commit"
  git -C "$tmp_wolfi" checkout --quiet FETCH_HEAD -- pipelines "${UPSTREAM}" 2>/dev/null || \
    git -C "$tmp_wolfi" checkout --quiet FETCH_HEAD -- pipelines
  rm -rf melange-work/pipelines
  cp -r "$tmp_wolfi/pipelines" melange-work/pipelines
  if [ -d "$tmp_wolfi/${UPSTREAM}" ]; then
    cp -r "$tmp_wolfi/${UPSTREAM}/." melange-work/
    echo "Copied ${UPSTREAM}/ companion files into melange-work/"
  fi
else
  echo "melange block has neither upstream nor bespoke set" >&2
  exit 1
fi

echo "Generating ephemeral melange signing key"
melange keygen melange-work/melange.rsa

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  MELANGE_ARCH="x86_64" ;;
  aarch64|arm64) MELANGE_ARCH="aarch64" ;;
  *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac

MELANGE_ARGS=(
  build melange-work/build.yaml
  --arch "$MELANGE_ARCH"
  --signing-key melange-work/melange.rsa
  --out-dir packages/repo
  --repository-append https://packages.wolfi.dev/os
  --keyring-append https://packages.wolfi.dev/os/wolfi-signing.rsa.pub
  --runner docker
)

if [ -d melange-work/pipelines ]; then
  MELANGE_ARGS+=(--pipeline-dirs melange-work/pipelines)
fi

if [ -n "$ENV_FILE" ]; then
  MELANGE_ARGS+=(--env-file "packages/overrides/${ENV_FILE}")
fi
if [ -n "$BUILD_OPTION" ]; then
  MELANGE_ARGS+=(--build-option "$BUILD_OPTION")
fi

echo "Running: melange ${MELANGE_ARGS[*]}"
melange "${MELANGE_ARGS[@]}"

echo ""
echo "Built packages:"
find packages/repo -type f -name '*.apk'
