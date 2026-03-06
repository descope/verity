#!/usr/bin/env bash
set -euo pipefail

# push-reports.sh — Push JSON files to the reports branch via the GitHub Contents API.
#
# Concurrent-safe: uses PUT with SHA for updates, no git clone needed.
# Includes retry logic for transient API failures and rate limiting.
#
# Usage:
#   push-reports.sh <remote-path> <local-file> [<remote-path> <local-file> ...]
#
# Examples:
#   # Copa: push pre/post scan reports
#   push-reports.sh reports/nginx/1.27/pre.json pre.json \
#                   reports/nginx/1.27/post.json post.json
#
#   # Integer: push build report
#   push-reports.sh reports/node/22/default/latest.json report.json
#
# Required env: GH_TOKEN, GITHUB_REPOSITORY

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GH_TOKEN:?GH_TOKEN is required}"

OWNER="${GITHUB_REPOSITORY%/*}"
REPO="${GITHUB_REPOSITORY#*/}"
BRANCH="reports"

# gh_api_retry — retry gh api calls on transient failures.
# Usage: gh_api_retry <max_attempts> <gh_api_args...>
gh_api_retry() {
  local max="${1}"; shift
  local attempt=1
  while true; do
    if gh api "$@" 2>/dev/null; then
      return 0
    fi
    local rc=$?
    if [ "$attempt" -ge "$max" ]; then
      echo "gh api failed after ${max} attempts (exit ${rc})" >&2
      return "$rc"
    fi
    local wait=$(( attempt * 5 + RANDOM % 5 ))
    echo "gh api attempt ${attempt} failed (exit ${rc}), retrying in ${wait}s..." >&2
    sleep "$wait"
    attempt=$(( attempt + 1 ))
  done
}

push_file() {
  local remote_path="$1"
  local local_file="$2"
  local timestamp
  timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  local message="chore: update ${remote_path} @ ${timestamp}"

  if [ ! -f "$local_file" ]; then
    echo "File not found: $local_file" >&2
    return 1
  fi

  # Write base64 content to a temp file — passing it as a shell argument
  # fails with "Argument list too long" for large Trivy reports.
  local tmpfile
  tmpfile=$(mktemp)
  # shellcheck disable=SC2064
  trap "rm -f '${tmpfile}'" RETURN
  base64 < "$local_file" | tr -d '\n' > "$tmpfile"

  # Get current file SHA (required for updates, absent for creates).
  local sha=""
  sha=$(gh_api_retry 3 "repos/${OWNER}/${REPO}/contents/${remote_path}?ref=${BRANCH}" \
           --jq '.sha') || sha=""

  if [ -n "$sha" ]; then
    gh_api_retry 5 --method PUT "repos/${OWNER}/${REPO}/contents/${remote_path}" \
      --field "message=${message}" \
      --field "content=@${tmpfile}" \
      --field "sha=${sha}" \
      --field "branch=${BRANCH}" \
      --silent
  else
    gh_api_retry 5 --method PUT "repos/${OWNER}/${REPO}/contents/${remote_path}" \
      --field "message=${message}" \
      --field "content=@${tmpfile}" \
      --field "branch=${BRANCH}" \
      --silent
  fi

  echo "✓ Pushed ${local_file} → ${BRANCH}/${remote_path}"
}

# Process pairs of <remote-path> <local-file>
if [ $# -lt 2 ] || [ $(( $# % 2 )) -ne 0 ]; then
  echo "Usage: push-reports.sh <remote-path> <local-file> [<remote-path> <local-file> ...]" >&2
  exit 1
fi

while [ $# -ge 2 ]; do
  push_file "$1" "$2"
  shift 2
done
