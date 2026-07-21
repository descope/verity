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
EVENT="${WAIT_EVENT:-}"
BATCH_ID="${WAIT_BATCH_ID:-}"
EXPECTED_RUNS="${WAIT_EXPECTED_RUNS:-}"

case "$LOOKBACK_HOURS" in
  ''|*[!0-9]*|0*) echo "WAIT_LOOKBACK_HOURS must be a positive integer" >&2; exit 2 ;;
esac
case "$TIMEOUT_SECONDS" in
  ''|*[!0-9]*|0*) echo "WAIT_TIMEOUT_SECONDS must be a positive integer" >&2; exit 2 ;;
esac
case "$INTERVAL_SECONDS" in
  ''|*[!0-9]*|0*) echo "WAIT_INTERVAL_SECONDS must be a positive integer" >&2; exit 2 ;;
esac
if [ -n "$EVENT" ] && [[ ! "$EVENT" =~ ^[a-z_]+$ ]]; then
  echo "WAIT_EVENT contains unsupported characters" >&2
  exit 2
fi
if [ -n "$BATCH_ID" ] && [[ ! "$BATCH_ID" =~ ^[0-9]+-[0-9]+$ ]]; then
  echo "WAIT_BATCH_ID must have the form RUN_ID-RUN_ATTEMPT" >&2
  exit 2
fi
if [ -n "$BATCH_ID" ] && [[ ! "$EXPECTED_RUNS" =~ ^[1-9][0-9]*$ ]]; then
  echo "WAIT_EXPECTED_RUNS must be a positive integer when WAIT_BATCH_ID is set" >&2
  exit 2
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI is required" >&2
  exit 2
fi

repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
cutoff="$(date -u -d "-${LOOKBACK_HOURS} hours" +%Y-%m-%dT%H:%M:%SZ)"
deadline=$((SECONDS + TIMEOUT_SECONDS))
selector="select(.created_at >= \"${cutoff}\")"
if [ -n "$EVENT" ]; then
  selector="${selector} | select(.event == \"${EVENT}\")"
fi
if [ -n "$BATCH_ID" ]; then
  selector="${selector} | select(.display_title | endswith(\" [batch ${BATCH_ID}]\"))"
fi

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
          --jq ".workflow_runs[] | ${selector} | [\"${workflow}\", .id, .status, .created_at, .html_url] | @tsv"
      done
    done | sort -u
  )"

  if [ -z "$active" ]; then
    echo "No active producer workflow runs remain."

    for workflow in "$@"; do
      if [ -n "$BATCH_ID" ]; then
        completed="$(
          gh api "repos/${repo}/actions/workflows/${workflow}/runs" \
            --method GET \
            --paginate \
            -f branch="${BRANCH}" \
            --jq ".workflow_runs[] | ${selector} | select(.status == \"completed\") | [.id, .run_attempt, .conclusion, .created_at, .html_url] | @tsv"
        )"
        completed_count="$(grep -c . <<< "$completed" || true)"
        if [ "$completed_count" -lt "$EXPECTED_RUNS" ]; then
          echo "Waiting for ${workflow} batch ${BATCH_ID}: ${completed_count}/${EXPECTED_RUNS} completed."
          break
        fi
        while IFS=$'\t' read -r run_id run_attempt conclusion created_at run_url; do
          if [ "$conclusion" != "success" ]; then
            echo "${workflow} batch ${BATCH_ID} run did not succeed: ${conclusion} (${run_url})" >&2
            exit 1
          fi
        done <<< "$completed"
        echo "All ${EXPECTED_RUNS} ${workflow} batch ${BATCH_ID} runs succeeded."
        continue
      fi

      latest="$(
        gh api "repos/${repo}/actions/workflows/${workflow}/runs" \
          --method GET \
          --paginate \
          -f branch="${BRANCH}" \
          --jq ".workflow_runs | map(${selector} | select(.status == \"completed\")) | sort_by(.created_at) | last | if . == null then empty else [.id, .run_attempt, .conclusion, .created_at, .html_url] | @tsv end"
      )"

      if [ -z "$latest" ]; then
        echo "Waiting for a completed ${workflow} producer run since ${cutoff}."
        break
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

    if [ -n "$BATCH_ID" ] && [ "${completed_count:-0}" -lt "$EXPECTED_RUNS" ]; then
      :
    elif [ -z "$BATCH_ID" ] && [ -z "${latest:-}" ]; then
      :
    else
      exit 0
    fi
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
