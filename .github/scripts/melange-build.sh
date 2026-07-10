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

validate_lock_path() {
  local label="$1" value="$2"

  if [[ ! "$value" =~ ^[A-Za-z0-9._/-]+$ ]]; then
    echo "${label} contains unsafe characters: '${value}'" >&2
    exit 1
  fi

  if [[ "$value" == /* ]] || [[ "$value" == *".."* ]] || [[ "$value" == *"//"* ]]; then
    echo "${label} must be a safe relative path without traversal: '${value}'" >&2
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

verify_locked_file() {
  local label="$1" path="$2" expected="$3" actual resolved expected_path
  if [ ! -f "$path" ] || [ -L "$path" ]; then
    echo "$label not found: $path" >&2
    exit 1
  fi
  resolved=$(realpath -e "$path")
  expected_path="$(pwd -P)/$path"
  if [ "$resolved" != "$expected_path" ]; then
    echo "$label must resolve within repository path: $path" >&2
    exit 1
  fi
  actual=$(sha256_file "$path")
  if [ "$actual" != "$expected" ]; then
    echo "sha256 mismatch for $label: expected $expected, got $actual" >&2
    exit 1
  fi
}

verify_regular_tree() {
  local label="$1" root="$2" invalid resolved expected_path

  if [ -L "$root" ]; then
    echo "$label root must be a real directory: $root" >&2
    exit 1
  fi
  if [ -e "$root" ] && [ ! -d "$root" ]; then
    echo "$label root must be a real directory: $root" >&2
    exit 1
  fi
  [ -d "$root" ] || return 0
  resolved=$(realpath -e "$root")
  expected_path="$(pwd -P)/$root"
  if [ "$resolved" != "$expected_path" ]; then
    echo "$label root must resolve within repository path: $root" >&2
    exit 1
  fi
  invalid=$(find "$root" -mindepth 1 ! -type d ! -type f -print -quit)
  if [ -n "$invalid" ]; then
    echo "$label contains non-regular file: $invalid" >&2
    exit 1
  fi
}

compare_locked_paths() {
  local label="$1" expected="$2" actual="$3"
  if ! diff -u "$expected" "$actual" >/dev/null; then
    echo "$label file set does not match lock manifest" >&2
    diff -u "$expected" "$actual" >&2 || true
    exit 1
  fi
}

stage_locked_recipe() {
  local pkg="$1" spec_dir="$2"
  local file expected_sha recipe_path sidecar_dir expected_paths actual_paths path sha

  file=$(jq -r --arg pkg "$pkg" '.packages[$pkg].file // empty' packages/upstream.lock.json)
  expected_sha=$(jq -r --arg pkg "$pkg" '.packages[$pkg].sha256 // empty' packages/upstream.lock.json)
  if [ -z "$file" ] || [ -z "$expected_sha" ]; then
    echo "Package '$pkg' is missing file or sha256 lock metadata" >&2
    exit 1
  fi
  validate_lock_path "recipe file" "$file"

  recipe_path="packages/bespoke/locked/$file"
  verify_locked_file "recipe $pkg" "$recipe_path" "$expected_sha"

  expected_paths=$(mktemp)
  actual_paths=$(mktemp)
  jq -r --arg pkg "$pkg" '.packages[$pkg].assets // {} | keys[]' packages/upstream.lock.json | LC_ALL=C sort >"$expected_paths"
  sidecar_dir="packages/bespoke/locked/$(basename "$file" .yaml)"
  verify_regular_tree "Recipe $pkg sidecar" "$sidecar_dir"
  if [ -d "$sidecar_dir" ]; then
    find "$sidecar_dir" -type f -print | sed 's#^packages/bespoke/locked/##' | LC_ALL=C sort >"$actual_paths"
  else
    : >"$actual_paths"
  fi
  compare_locked_paths "Recipe $pkg sidecar" "$expected_paths" "$actual_paths"

  while IFS=$'\t' read -r path sha; do
    [ -n "$path" ] || continue
    validate_lock_path "recipe asset" "$path"
    verify_locked_file "recipe asset $path" "packages/bespoke/locked/$path" "$sha"
  done < <(jq -r --arg pkg "$pkg" '.packages[$pkg].assets // {} | to_entries[] | [.key, .value] | @tsv' packages/upstream.lock.json)
  rm -f "$expected_paths" "$actual_paths"

  rm -rf "$spec_dir"
  mkdir -p "$spec_dir"
  cp "$recipe_path" "$spec_dir/build.yaml"
  if [ -d "$sidecar_dir" ]; then
    cp -R "$sidecar_dir/." "$spec_dir/"
  fi
}

stage_locked_pipelines() {
  local expected_paths actual_paths path sha

  expected_paths=$(mktemp)
  actual_paths=$(mktemp)
  jq -r '.pipeline_files // {} | keys[]' packages/upstream.lock.json | LC_ALL=C sort >"$expected_paths"
  verify_regular_tree "Shared pipeline" packages/pipelines
  if [ -d packages/pipelines ]; then
    find packages/pipelines -type f -print | sed 's#^packages/pipelines/##' | LC_ALL=C sort >"$actual_paths"
  else
    : >"$actual_paths"
  fi
  compare_locked_paths "Shared pipeline" "$expected_paths" "$actual_paths"

  while IFS=$'\t' read -r path sha; do
    [ -n "$path" ] || continue
    validate_lock_path "pipeline file" "$path"
    verify_locked_file "pipeline $path" "packages/pipelines/$path" "$sha"
  done < <(jq -r '.pipeline_files // {} | to_entries[] | [.key, .value] | @tsv' packages/upstream.lock.json)
  rm -f "$expected_paths" "$actual_paths"

  rm -rf melange-work/pipelines
  if [ -d packages/pipelines ]; then
    mkdir -p melange-work/pipelines
    cp -R packages/pipelines/. melange-work/pipelines/
  fi
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

elif [ -n "${UPSTREAM:-}" ]; then
  if [[ ! "$UPSTREAM" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "upstream value contains unsafe characters: '${UPSTREAM}'" >&2
    exit 1
  fi
  spec_dir="melange-work/specs/${UPSTREAM}"
  echo "Using locked bespoke melange YAML for ${UPSTREAM}"
  stage_locked_recipe "$UPSTREAM" "$spec_dir"
else
  echo "Neither BESPOKE_JSON nor UPSTREAM is set" >&2
  exit 1
fi

stage_locked_pipelines

if [ "${STAGE_ONLY:-false}" = "true" ]; then
  exit 0
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

cp melange-work/melange.rsa.pub packages/repo/

for index in packages/repo/*/APKINDEX.tar.gz; do
  [ -f "$index" ] || continue
  echo "Signing APKINDEX: $index"
  melange sign-index --signing-key melange-work/melange.rsa "$index" --force
done
