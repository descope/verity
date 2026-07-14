#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "Usage: wait-for-workflows.sh <workflow-file-or-name>..." >&2
  exit 2
fi

BRANCH="${WAIT_BRANCH:-${GITHUB_REF_NAME:-main}}"
LOOKBACK_HOURS="${WAIT_LOOKBACK_HOURS:-8}"
TIMEOUT_SECONDS="${WAIT_TIMEOUT_SECONDS:-7200}"
INTERVAL_SECONDS="${WAIT_INTERVAL_SECONDS:-60}"

case "$LOOKBACK_HOURS" in
  ''|*[!0-9]*|0*) echo "WAIT_LOOKBACK_HOURS must be a positive integer" >&2; exit 2 ;;
esac
case "$TIMEOUT_SECONDS" in
  ''|*[!0-9]*|0*) echo "WAIT_TIMEOUT_SECONDS must be a positive integer" >&2; exit 2 ;;
esac
case "$INTERVAL_SECONDS" in
  ''|*[!0-9]*|0*) echo "WAIT_INTERVAL_SECONDS must be a positive integer" >&2; exit 2 ;;
esac

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI is required" >&2
  exit 2
fi

repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
cutoff="$(date -u -d "-${LOOKBACK_HOURS} hours" +%Y-%m-%dT%H:%M:%SZ)"
deadline=$((SECONDS + TIMEOUT_SECONDS))

echo "Waiting for active workflow runs on ${BRANCH} since ${cutoff}: $*"

while true; do
  active="$(
    for workflow in "$@"; do
      for status in queued in_progress requested waiting; do
        gh api "repos/${repo}/actions/workflows/${workflow}/runs" \
          --method GET \
          --paginate \
          -f branch="${BRANCH}" \
          -f status="${status}" \
          --jq ".workflow_runs[] | select(.created_at >= \"${cutoff}\") | [\"${workflow}\", .id, .status, .created_at, .html_url] | @tsv"
      done
    done | sort -u
  )"

  if [ -z "$active" ]; then
    echo "No active producer workflow runs remain."

    for workflow in "$@"; do
      latest="$(
        gh api "repos/${repo}/actions/workflows/${workflow}/runs" \
          --method GET \
          --paginate \
          -f branch="${BRANCH}" \
          --jq ".workflow_runs | map(select(.created_at >= \"${cutoff}\" and .status == \"completed\")) | sort_by(.created_at) | last | if . == null then empty else [.id, .run_attempt, .conclusion, .created_at, .html_url] | @tsv end"
      )"

      if [ -z "$latest" ]; then
        echo "No completed ${workflow} producer run found since ${cutoff}." >&2
        exit 1
      fi

      IFS=$'\t' read -r run_id run_attempt conclusion created_at run_url <<< "$latest"
      if [ "$conclusion" != "success" ]; then
        echo "Latest ${workflow} producer run did not succeed: ${conclusion} (${run_url})" >&2
        exit 1
      fi

      output_name="${workflow%.yaml}_batch_id"
      output_name="${output_name//-/_}"
      if [ -n "${GITHUB_OUTPUT:-}" ]; then
        echo "${output_name}=${run_id}-${run_attempt}" >> "$GITHUB_OUTPUT"
      fi
      echo "Latest ${workflow} producer succeeded at ${created_at}: ${run_id}-${run_attempt}"
    done

    exit 0
  fi

  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "Timed out waiting for producer workflows:" >&2
    printf '%s\n' "$active" >&2
    exit 1
  fi

  echo "Still waiting for producer workflows:"
  printf '%s\n' "$active"
  sleep "$INTERVAL_SECONDS"
done
