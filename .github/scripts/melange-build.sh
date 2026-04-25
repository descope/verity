#!/bin/bash
set -euo pipefail
#
# Resolves the melange YAML source (bespoke or upstream), generates
# an ephemeral signing key, and builds a single-arch APK package.
# Expects BESPOKE/UPSTREAM, ENV_FILE, and BUILD_OPTION env vars.

mkdir -p melange-work

if [ -n "${BESPOKE:-}" ]; then
  cp "packages/bespoke/${BESPOKE}" melange-work/build.yaml

  # Bespoke builds may use Wolfi-specific pipelines (e.g. py/pip-build-install).
  # Fetch the pipelines/ tree from the pinned wolfi_commit so they resolve.
  commit=$(jq -r '.wolfi_commit' packages/upstream.lock.json)
  if [ "$commit" != "null" ] && [ -n "$commit" ]; then
    echo "Fetching wolfi pipelines/ at commit ${commit} for bespoke build"
    tmp_wolfi=$(mktemp -d)
    trap 'rm -rf "$tmp_wolfi"' EXIT
    git -C "$tmp_wolfi" init --quiet
    git -C "$tmp_wolfi" remote add origin "https://github.com/wolfi-dev/os.git"
    git -C "$tmp_wolfi" sparse-checkout set --no-cone pipelines
    git -C "$tmp_wolfi" fetch --quiet --depth 1 --filter=blob:none origin "$commit"
    git -C "$tmp_wolfi" checkout --quiet FETCH_HEAD -- pipelines
    rm -rf melange-work/pipelines
    cp -r "$tmp_wolfi/pipelines" melange-work/pipelines
  fi
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
  curl -fsSL "$url" -o melange-work/build.yaml.tmp
  actual_sha=$(sha256sum melange-work/build.yaml.tmp | awk '{print $1}')
  if [ "$actual_sha" != "$expected_sha" ]; then
    echo "sha256 mismatch for ${UPSTREAM}: expected ${expected_sha}, got ${actual_sha}" >&2
    rm -f melange-work/build.yaml.tmp
    exit 1
  fi
  mv melange-work/build.yaml.tmp melange-work/build.yaml

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
    cp -r "$tmp_wolfi/${UPSTREAM}/." melange-work/
    echo "Copied ${UPSTREAM}/ companion files into melange-work/"
  fi
else
  echo "Neither BESPOKE nor UPSTREAM is set" >&2
  exit 1
fi

melange keygen melange-work/melange.rsa

MELANGE_ARGS=(
  build melange-work/build.yaml
  --arch x86_64
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
