#!/usr/bin/env bash
set -euo pipefail

# Retry registry-facing commands to absorb transient GHCR 5xx failures.
# Usage: retry-registry-command.sh "label" command [args...]

if [ "$#" -lt 2 ]; then
  echo "::error::usage: $0 <label> <command> [args...]" >&2
  exit 2
fi

LABEL="$1"
shift

MAX_ATTEMPTS="${REGISTRY_COMMAND_ATTEMPTS:-4}"
BASE_DELAY_SECONDS="${REGISTRY_COMMAND_BASE_DELAY_SECONDS:-10}"

case "$MAX_ATTEMPTS" in
  ''|*[!0-9]*|0|0[0-9]*)
    echo "::error::REGISTRY_COMMAND_ATTEMPTS must be a positive integer" >&2
    exit 2
    ;;
esac

case "$BASE_DELAY_SECONDS" in
  ''|*[!0-9]*|0|0[0-9]*)
    echo "::error::REGISTRY_COMMAND_BASE_DELAY_SECONDS must be a positive integer" >&2
    exit 2
    ;;
esac

for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  echo "::group::${LABEL} (attempt ${attempt}/${MAX_ATTEMPTS})" >&2
  rc=0
  if "$@"; then
    echo "::endgroup::" >&2
    exit 0
  else
    rc=$?
    echo "::endgroup::" >&2
  fi

  if [ "$attempt" -ge "$MAX_ATTEMPTS" ]; then
    echo "::error::${LABEL} failed after ${MAX_ATTEMPTS} attempts (exit ${rc})" >&2
    exit "$rc"
  fi

  wait_seconds=$(( BASE_DELAY_SECONDS * attempt + RANDOM % BASE_DELAY_SECONDS ))
  echo "${LABEL} failed (exit ${rc}); retrying in ${wait_seconds}s..." >&2
  sleep "$wait_seconds"
done
