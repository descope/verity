#!/usr/bin/env bash
# push-catalog.sh — push catalog.json to the root of the reports branch.
#
# Usage: push-catalog.sh <catalog-file>
#
# Required env: GH_TOKEN, GITHUB_REPOSITORY
set -euo pipefail

CATALOG_FILE="${1:?catalog file required}"
REPO="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY not set}"
BRANCH="reports"
API="https://api.github.com/repos/${REPO}/contents"
PATH_IN_REPO="catalog.json"

if [ ! -f "$CATALOG_FILE" ]; then
  echo "Catalog file not found: $CATALOG_FILE" >&2
  exit 1
fi

# Write base64 content to a temp file to avoid argument list too long error.
CONTENT_FILE=$(mktemp)
BODY_FILE=$(mktemp)
trap 'rm -f "$CONTENT_FILE" "$BODY_FILE"' EXIT
base64 -w0 < "$CATALOG_FILE" | tr -d '\n' > "$CONTENT_FILE"

# Fetch existing SHA (needed for update vs create).
existing_sha=""
response=$(curl -sf \
  -H "Authorization: Bearer ${GH_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  "${API}/${PATH_IN_REPO}?ref=${BRANCH}" 2>/dev/null || true)

if [ -n "$response" ]; then
  existing_sha=$(echo "$response" | jq -r '.sha // empty')
fi

timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
message="catalog: regenerate @ ${timestamp}"

if [ -n "$existing_sha" ]; then
  jq -n \
    --arg message "$message" \
    --rawfile content "$CONTENT_FILE" \
    --arg sha "$existing_sha" \
    --arg branch "$BRANCH" \
    '{message: $message, content: $content, sha: $sha, branch: $branch}' > "$BODY_FILE"
else
  jq -n \
    --arg message "$message" \
    --rawfile content "$CONTENT_FILE" \
    --arg branch "$BRANCH" \
    '{message: $message, content: $content, branch: $branch}' > "$BODY_FILE"
fi

curl -sf \
  -X PUT \
  -H "Authorization: Bearer ${GH_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  -H "Content-Type: application/json" \
  --data @"$BODY_FILE" \
  "${API}/${PATH_IN_REPO}" > /dev/null

echo "Pushed ${CATALOG_FILE} → ${BRANCH}/${PATH_IN_REPO}"
