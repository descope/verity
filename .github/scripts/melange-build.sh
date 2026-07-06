#!/bin/bash
set -euo pipefail
mkdir -p melange-work/specs

validate_filename() {
  local label="$1" value="$2"

  if [[ ! "$value" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "${label} contains unsafe characters: '${value}'" >&2
    exit 1
  fi

  if [[ "$value" == *".."* ]]; then
    echo "${label} must not contain path traversal sequences ('..'): '${value}'" >&2
    exit 1
  fi
}

fetch_wolfi_pipelines() {
  commit=$(jq -r '.wolfi_commit' packages/upstream.lock.json)
  if [ "$commit" = "null" ] || [ -z "$commit" ]; then
    echo "wolfi_commit missing or null in packages/upstream.lock.json" >&2
    exit 1
  fi

  echo "Fetching wolfi pipelines/ at commit ${commit}"
  tmp_wolfi=$(mktemp -d)
  trap 'rm -rf "$tmp_wolfi"' EXIT
  git -C "$tmp_wolfi" init --quiet
  git -C "$tmp_wolfi" remote add origin "https://github.com/wolfi-dev/os.git"
  git -C "$tmp_wolfi" sparse-checkout set --no-cone pipelines
  git -C "$tmp_wolfi" fetch --quiet --depth 1 --filter=blob:none origin "$commit"
  git -C "$tmp_wolfi" checkout --quiet FETCH_HEAD -- pipelines
  rm -rf melange-work/pipelines
  cp -r "$tmp_wolfi/pipelines" melange-work/pipelines
}

build_one() {
  local build_yaml="$1"
  local build_arch="${BUILD_ARCH:-x86_64}"

  MELANGE_ARGS=(
    build "$build_yaml"
    --arch "$build_arch"
    --signing-key melange-work/melange.rsa
    --out-dir packages/repo
    --repository-append https://packages.wolfi.dev/os
    --keyring-append https://packages.wolfi.dev/os/wolfi-signing.rsa.pub
    --runner docker
  )

  if [ -d melange-work/pipelines ]; then
    MELANGE_ARGS+=(--pipeline-dirs melange-work/pipelines)
  fi

  if [ -n "${ENV_FILE:-}" ]; then
    MELANGE_ARGS+=(--env-file "packages/overrides/${ENV_FILE}")
  fi
  if [ -n "${BUILD_OPTION:-}" ]; then
    MELANGE_ARGS+=(--build-option "$BUILD_OPTION")
  fi

  echo "Running: melange ${MELANGE_ARGS[*]}"
  melange "${MELANGE_ARGS[@]}"
}

BESPOKE_JSON=${BESPOKE_JSON:-[]}

if [ "$BESPOKE_JSON" != '[]' ]; then
  while IFS= read -r bespoke; do
    validate_filename "BESPOKE" "$bespoke"
    if [ ! -f "packages/bespoke/${bespoke}" ]; then
      echo "Bespoke build file not found: packages/bespoke/${bespoke}" >&2
      exit 1
    fi
    spec_dir="melange-work/specs/${bespoke}"
    rm -rf "$spec_dir"
    mkdir -p "$spec_dir"
    cp "packages/bespoke/${bespoke}" "$spec_dir/build.yaml"
  done < <(printf '%s' "$BESPOKE_JSON" | jq -r '.[]')

  fetch_wolfi_pipelines
elif [ -n "${UPSTREAM:-}" ]; then
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
  spec_dir="melange-work/specs/${UPSTREAM}"
  rm -rf "$spec_dir"
  mkdir -p "$spec_dir"
  curl -fsSL "$url" -o "$spec_dir/build.yaml.tmp"
  actual_sha=$(sha256sum "$spec_dir/build.yaml.tmp" | awk '{print $1}')
  if [ "$actual_sha" != "$expected_sha" ]; then
    echo "sha256 mismatch for ${UPSTREAM}: expected ${expected_sha}, got ${actual_sha}" >&2
    rm -f "$spec_dir/build.yaml.tmp"
    exit 1
  fi
  mv "$spec_dir/build.yaml.tmp" "$spec_dir/build.yaml"

  echo "Fetching wolfi pipelines/ and ${UPSTREAM}/ companion dir at commit ${commit}"
  tmp_wolfi=$(mktemp -d)
  trap 'rm -rf "$tmp_wolfi"' EXIT
  git -C "$tmp_wolfi" init --quiet
  git -C "$tmp_wolfi" remote add origin "https://github.com/wolfi-dev/os.git"

  if [[ ! "$UPSTREAM" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "upstream value contains unsafe characters: '${UPSTREAM}'" >&2
    exit 1
  fi

  git -C "$tmp_wolfi" sparse-checkout set --no-cone pipelines "${UPSTREAM}"
  git -C "$tmp_wolfi" fetch --quiet --depth 1 --filter=blob:none origin "$commit"
  git -C "$tmp_wolfi" checkout --quiet FETCH_HEAD -- pipelines "${UPSTREAM}" 2>/dev/null || \
    git -C "$tmp_wolfi" checkout --quiet FETCH_HEAD -- pipelines
  rm -rf melange-work/pipelines
  cp -r "$tmp_wolfi/pipelines" melange-work/pipelines
  if [ -d "$tmp_wolfi/${UPSTREAM}" ]; then
    cp -r "$tmp_wolfi/${UPSTREAM}/." "$spec_dir/"
  fi
else
  echo "Neither BESPOKE_JSON nor UPSTREAM is set" >&2
  exit 1
fi

melange keygen melange-work/melange.rsa

shopt -s nullglob
builds=(melange-work/specs/*/build.yaml)
if [ ${#builds[@]} -eq 0 ]; then
  echo "No melange build YAMLs staged in melange-work/specs" >&2
  exit 1
fi

for build_yaml in "${builds[@]}"; do
  build_one "$build_yaml"
done
