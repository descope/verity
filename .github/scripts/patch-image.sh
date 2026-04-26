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
#   GO_VCS_URL     Explicit Go module VCS URL for stripped/distroless Go
#                  binaries. Flows through verity patch → copa's
#                  types.Options.GoVCSURL (currently sourced from a go.mod
#                  replace directive → verity-org/copacetic feat/go-vcs-
#                  resolution; the replace directive is dropped once
#                  upstream copa PR #1546 merges). The retry branch below
#                  handles the residual case where the Go rebuild still
#                  fails, falling back to OS-only patches. The retry also
#                  triggers when the patch log shows a Go-rebuild failure
#                  even if GO_VCS_URL was not set (e.g. cockroachdb's
#                  /cockroach/cockroach binary is at a non-standard path
#                  copa's discovery doesn't walk).

: "${PLATFORM:?PLATFORM is required}"
: "${SOURCE:?SOURCE is required}"
: "${IMAGE_NAME:?IMAGE_NAME is required}"
: "${STAGING_REGISTRY:?STAGING_REGISTRY is required}"

# Capture start time for duration metric. Trap on EXIT emits GitHub Actions
# step outputs (exit-code, duration-seconds, staging-digest) on every code
# path — success, PATCH_EXIT failure, and RETRY_EXIT failure.
START_SECONDS=$SECONDS
EMITTED_DIGEST=""
emit_outputs() {
  local exit_code=$?
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    {
      echo "exit-code=${exit_code}"
      echo "duration-seconds=$((SECONDS - START_SECONDS))"
      if [[ "$exit_code" -eq 0 && -n "$EMITTED_DIGEST" ]]; then
        echo "staging-digest=${EMITTED_DIGEST}"
      fi
    } >> "$GITHUB_OUTPUT"
  fi
}
trap emit_outputs EXIT

PLATFORM_ARCH=$(echo "$PLATFORM" | cut -d/ -f2)

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

_is_go_rebuild_failure() {
  # Pattern set covers every "go package upgrade operation failed" terminal
  # state copa surfaces today:
  #   - "no go.mod files detected ..." → distroless probe (kyverno, cert-mgr)
  #   - "no Go binaries detected ..."  → non-standard binary path (cockroach)
  #   - "no binaries were successfully rebuilt" → missing source repo per binary (mongodb tools)
  #   - "copa_discover_build.sh ... did not complete successfully" → go build crash (prom-config-reloader)
  #   - 'exec: "sh": executable file not found' → distroless image with no shell
  grep -qE 'go package upgrade operation failed|no go\.mod files detected|no Go binaries detected|no binaries were successfully rebuilt|copa_discover_build\.sh.*did not complete successfully|exec: "sh": executable file not found' "$PATCH_LOG"
}

if [ "$PATCH_EXIT" -ne 0 ]; then
  if grep -q 'no package updates found' "$PATCH_LOG"; then
    echo "No package updates found — image is already clean"
  elif [ -n "${GO_VCS_URL:-}" ] || _is_go_rebuild_failure; then
    # Retry triggers in two cases:
    #   1. GO_VCS_URL was set (catalog entry expects a rebuildable Go binary).
    #   2. The patch log shows a Go-rebuild failure pattern even without
    #      GO_VCS_URL (e.g. cockroach's non-standard binary path, mongo-tools
    #      stripped without buildinfo, prom-config-reloader's discover-build
    #      script crashing on a real go build error).
    # In both cases we retry with --pkg-types os to drop language managers
    # entirely: OS CVEs still get patched; Go/library CVEs remain unfixed
    # for this run and surface in the next preflight scan.
    if [ -n "${GO_VCS_URL:-}" ]; then
      echo "::warning::Patch failed for Go-rebuild image, retrying with OS-only patches"
    else
      echo "::warning::Patch hit a Go-rebuild failure (no GO_VCS_URL set); retrying with OS-only patches"
    fi
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

# Resolve final staging digest for GHA step output (consumed by trap).
EMITTED_DIGEST=$(crane digest "$PLATFORM_TAG" 2>/dev/null || echo "")

echo "Patched platform-specific image: $PLATFORM_TAG"
