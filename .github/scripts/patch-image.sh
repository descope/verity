#!/usr/bin/env bash
set -euo pipefail

# Patches a single platform-specific image using Copa and pushes to the staging registry.
# Falls back to crane copy if Copa finds no OS package updates (exits 0 but skips push).
# Required env vars: PLATFORM, SOURCE, IMAGE_NAME, STAGING_REGISTRY

: "${PLATFORM:?PLATFORM is required}"
: "${SOURCE:?SOURCE is required}"
: "${IMAGE_NAME:?IMAGE_NAME is required}"
: "${STAGING_REGISTRY:?STAGING_REGISTRY is required}"

# Extract platform arch for tag suffix (linux/amd64 -> amd64)
PLATFORM_ARCH=$(echo "$PLATFORM" | cut -d/ -f2)

# Extract source tag (e.g., nginx:1.29.4 -> 1.29.4)
SOURCE_TAG=$(echo "$SOURCE" | cut -d: -f2)

# Sanitize image name: chart images contain registry paths with slashes (invalid in OCI tags)
SAFE_IMAGE=$(echo "$IMAGE_NAME" | tr '/: ' '---')
PLATFORM_TAG="${STAGING_REGISTRY}:${SAFE_IMAGE}-${SOURCE_TAG}-${PLATFORM_ARCH}"

# Construct report filename (match how scan.go creates them)
REPORT_FILE="reports/$(echo "$SOURCE" | sed 's/[\/:]/_/g').json"

echo "Using report file: $REPORT_FILE"

# Copa patches the native platform of this runner (amd64 or arm64).
# --pkg-types os,library patches both OS packages and app-level deps (pip, npm)
# --library-patch-level major allows major version bumps for library fixes
COPA_LOG=$(mktemp)
set +e
COPA_ARGS=(
  --image "$SOURCE"
  --tag "$PLATFORM_TAG"
  --report "$REPORT_FILE"
  --pkg-types "os,library"
  --library-patch-level major
  --toolchain-patch-level patch
  --push
  --addr buildx://copa-builder
  --timeout 30m
)
if [ -n "${GO_VCS_URL:-}" ]; then
  COPA_ARGS+=(--go-vcs-url "$GO_VCS_URL")
fi
copa patch "${COPA_ARGS[@]}" 2>&1 | tee "$COPA_LOG"
COPA_EXIT=${PIPESTATUS[0]}
set -e

if [ "$COPA_EXIT" -ne 0 ]; then
  if grep -q 'no package updates found' "$COPA_LOG"; then
    echo "No package updates found — image is already clean"
  elif [ -n "${GO_VCS_URL:-}" ]; then
    # Go binary rebuild failed — retry with OS-only patches.
    # Copa still attempts Go rebuild even without --go-vcs-url (uses VCS
    # info from the binary). Using --pkg-types os skips language managers
    # entirely. Some Go/library CVEs remain but OS packages get patched.
    echo "::warning::Copa failed with --go-vcs-url, retrying without Go rebuild"
    RETRY_ARGS=(
      --image "$SOURCE"
      --tag "$PLATFORM_TAG"
      --report "$REPORT_FILE"
      --pkg-types "os"
      --push
      --addr buildx://copa-builder
      --timeout 30m
    )
    set +e
    copa patch "${RETRY_ARGS[@]}" 2>&1 | tee "$COPA_LOG"
    RETRY_EXIT=${PIPESTATUS[0]}
    set -e
    if [ "$RETRY_EXIT" -ne 0 ]; then
      if grep -q 'no package updates found' "$COPA_LOG"; then
        echo "No package updates found on retry — image is already clean"
      else
        rm -f "$COPA_LOG"
        exit "$RETRY_EXIT"
      fi
    fi
  else
    rm -f "$COPA_LOG"
    exit "$COPA_EXIT"
  fi
fi
rm -f "$COPA_LOG"

# When Copa finds no OS package updates it exits 0 but does not push.
# Copy the source image to the staging tag so the combine step can build
# the multi-platform manifest regardless of whether patches were applied.
crane digest "$PLATFORM_TAG" > /dev/null 2>&1 \
  || crane copy --platform "$PLATFORM" "$SOURCE" "$PLATFORM_TAG"

echo "Patched platform-specific image: $PLATFORM_TAG"
