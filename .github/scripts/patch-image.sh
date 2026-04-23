#!/usr/bin/env bash
set -euo pipefail

# Patches a single platform-specific image using `verity patch` (which imports
# Copa as a Go library — see internal/patch and cmd/patch.go). Pushes to the
# staging registry. Falls back to `crane copy` if Copa finds no OS package
# updates (verity patch exits 0 but does not push).
#
# Required env vars: PLATFORM, SOURCE, IMAGE_NAME, STAGING_REGISTRY
#
# Optional env vars:
#   GO_VCS_URL     Explicit Go module VCS URL for stripped/distroless binaries.
#                  Currently a no-op at the Go layer (upstream copa PR #1546
#                  is still open); verity patch logs a warning when set. Go
#                  binary rebuilds still work via copa's embedded buildinfo
#                  auto-detect on non-stripped binaries, with the retry below
#                  falling back to OS-only patches if the rebuild fails.

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

# verity patch wraps copa's pkg/patch.Patch. It accepts the same flag surface
# the legacy `copa patch` did, so the arg list below is largely unchanged.
# --pkg-types os,library patches both OS packages and app-level deps (pip, npm)
# --library-patch-level major allows major version bumps for library fixes
PATCH_LOG=$(mktemp)
set +e
PATCH_ARGS=(
  --image "$SOURCE"
  --tag "$PLATFORM_TAG"
  --report "$REPORT_FILE"
  --pkg-types "os,library"
  --library-patch-level major
  --toolchain-patch-level patch
  --push
  --buildkit-addr buildx://copa-builder
  --timeout 30m
)
if [ -n "${GO_VCS_URL:-}" ]; then
  PATCH_ARGS+=(--go-vcs-url "$GO_VCS_URL")
fi
./verity patch "${PATCH_ARGS[@]}" 2>&1 | tee "$PATCH_LOG"
PATCH_EXIT=${PIPESTATUS[0]}
set -e

if [ "$PATCH_EXIT" -ne 0 ]; then
  if grep -q 'no package updates found' "$PATCH_LOG"; then
    echo "No package updates found — image is already clean"
  elif [ -n "${GO_VCS_URL:-}" ]; then
    # The retry branch only triggers when GO_VCS_URL is set because that
    # flag marks catalog entries with a rebuildable Go binary — i.e., the
    # images most likely to fail on the Go-rebuild code path. When the
    # initial --pkg-types os,library attempt fails on such an image, we
    # retry with --pkg-types os to drop language managers entirely: OS
    # CVEs still get patched; Go/library CVEs remain unfixed for this run.
    #
    # Historical note: with the legacy verity-org/copacetic fork, passing
    # --go-vcs-url gave copa an explicit VCS URL override. Upstream copa
    # (currently pinned) doesn't yet expose that option; verity patch
    # accepts --go-vcs-url for flag compatibility but treats it as a
    # no-op pending upstream PR #1546. Once that merges, --go-vcs-url
    # will resume being wired through; the retry gate stays the same.
    echo "::warning::Patch failed for Go-rebuild image, retrying with OS-only patches"
    RETRY_ARGS=(
      --image "$SOURCE"
      --tag "$PLATFORM_TAG"
      --report "$REPORT_FILE"
      --pkg-types "os"
      --push
      --buildkit-addr buildx://copa-builder
      --timeout 30m
    )
    set +e
    ./verity patch "${RETRY_ARGS[@]}" 2>&1 | tee "$PATCH_LOG"
    RETRY_EXIT=${PIPESTATUS[0]}
    set -e
    if [ "$RETRY_EXIT" -ne 0 ]; then
      if grep -q 'no package updates found' "$PATCH_LOG"; then
        echo "No package updates found on retry — image is already clean"
      else
        rm -f "$PATCH_LOG"
        exit "$RETRY_EXIT"
      fi
    fi
  else
    rm -f "$PATCH_LOG"
    exit "$PATCH_EXIT"
  fi
fi
rm -f "$PATCH_LOG"

# When Copa finds no OS package updates it exits 0 but does not push.
# Copy the source image to the staging tag so the combine step can build
# the multi-platform manifest regardless of whether patches were applied.
crane digest "$PLATFORM_TAG" > /dev/null 2>&1 \
  || crane copy --platform "$PLATFORM" "$SOURCE" "$PLATFORM_TAG"

echo "Patched platform-specific image: $PLATFORM_TAG"
