#!/usr/bin/env bash
set -euo pipefail

# Retry registry login to absorb transient GHCR token endpoint timeouts.
# Required env: DOCKER_REGISTRY, DOCKER_USERNAME, DOCKER_PASSWORD

: "${DOCKER_REGISTRY:?DOCKER_REGISTRY is required}"
: "${DOCKER_USERNAME:?DOCKER_USERNAME is required}"
: "${DOCKER_PASSWORD:?DOCKER_PASSWORD is required}"

MAX_ATTEMPTS="${DOCKER_LOGIN_ATTEMPTS:-4}"
TIMEOUT_SECONDS="${DOCKER_LOGIN_TIMEOUT_SECONDS:-45}"

case "$MAX_ATTEMPTS" in
  ''|*[!0-9]*|0|0[0-9]*)
    echo "::error::DOCKER_LOGIN_ATTEMPTS must be a positive integer"
    exit 2
    ;;
esac

case "$TIMEOUT_SECONDS" in
  ''|*[!0-9]*|0|0[0-9]*)
    echo "::error::DOCKER_LOGIN_TIMEOUT_SECONDS must be a positive integer"
    exit 2
    ;;
esac

for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  echo "Logging into ${DOCKER_REGISTRY} (attempt ${attempt}/${MAX_ATTEMPTS})..."
  rc=0
  if printf '%s' "$DOCKER_PASSWORD" \
    | timeout "${TIMEOUT_SECONDS}s" docker login "$DOCKER_REGISTRY" \
        --username "$DOCKER_USERNAME" \
        --password-stdin; then
    echo "✓ Logged into ${DOCKER_REGISTRY}"
    exit 0
  else
    rc=$?
  fi

  if [ "$attempt" -ge "$MAX_ATTEMPTS" ]; then
    echo "::error::Docker login to ${DOCKER_REGISTRY} failed after ${MAX_ATTEMPTS} attempts (exit ${rc})"
    exit "$rc"
  fi

  wait_seconds=$(( attempt * 10 + RANDOM % 10 ))
  echo "Docker login failed (exit ${rc}); retrying in ${wait_seconds}s..."
  sleep "$wait_seconds"
done
