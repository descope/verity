#!/usr/bin/env bash
# push-reports.sh — push a build report JSON to the reports branch.
#
# Usage: push-reports.sh <image> <variant> <report-file>
#
# Uses the GitHub Contents API so multiple concurrent jobs can write
# different paths without conflicts (no git clone needed).
#
# Required env: GH_TOKEN, GITHUB_REPOSITORY
set -euo pipefail

IMAGE="${1:?image name required}"
VARIANT="${2:?variant name required}"
REPORT_FILE="${3:?report file required}"

REPO="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY not set}"
BRANCH="reports"
API="https://api.github.com/repos/${REPO}/contents"
REPORT_PATH="reports/${IMAGE}/${VARIANT}/latest.json"

if [ ! -f "$REPORT_FILE" ]; then
  echo "Report file not found: $REPORT_FILE" >&2
  exit 1
fi

CONTENT=$(base64 -w0 < "$REPORT_FILE")

# curl_with_retry — retry on transient HTTP errors (exit code 22 = 4xx/5xx).
# Usage: curl_with_retry <max_attempts> <curl_args...>
curl_with_retry() {
  local max="${1}"; shift
  local attempt=1
  while true; do
    if curl "$@"; then
      return 0
    fi
    local rc=$?
    if [ "$attempt" -ge "$max" ]; then
      echo "curl failed after ${max} attempts (exit ${rc})" >&2
      return "$rc"
    fi
    local wait=$(( attempt * 5 + RANDOM % 5 ))
    echo "curl attempt ${attempt} failed (exit ${rc}), retrying in ${wait}s..." >&2
    sleep "$wait"
    attempt=$(( attempt + 1 ))
  done
}

# Check if file already exists (need its SHA for update).
existing_sha=""
response=$(curl_with_retry 5 -sf \
  -H "Authorization: Bearer ${GH_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  "${API}/${REPORT_PATH}?ref=${BRANCH}" 2>/dev/null || true)

if [ -n "$response" ]; then
  existing_sha=$(echo "$response" | jq -r '.sha // empty')
fi

# Build the request body.
timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
message="report: ${IMAGE}/${VARIANT} @ ${timestamp}"

if [ -n "$existing_sha" ]; then
  body=$(jq -n \
    --arg message "$message" \
    --arg content "$CONTENT" \
    --arg sha "$existing_sha" \
    --arg branch "$BRANCH" \
    '{message: $message, content: $content, sha: $sha, branch: $branch}')
else
  body=$(jq -n \
    --arg message "$message" \
    --arg content "$CONTENT" \
    --arg branch "$BRANCH" \
    '{message: $message, content: $content, branch: $branch}')
fi

# PUT to create or update (retry up to 5 times on rate-limiting).
curl_with_retry 5 -sf \
  -X PUT \
  -H "Authorization: Bearer ${GH_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  -H "Content-Type: application/json" \
  --data "$body" \
  "${API}/${REPORT_PATH}" > /dev/null

echo "Pushed report to ${BRANCH}/${REPORT_PATH}"
