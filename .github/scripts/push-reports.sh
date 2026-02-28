#!/usr/bin/env bash
set -euo pipefail

# Pushes pre/post Trivy scan reports to the reports branch via the GitHub API.
# Concurrent-safe: uses PUT with SHA for updates, no git operations.
# Usage: push-reports.sh <name> <tag> <pre-json> <post-json>
# Required env: GH_TOKEN (used by gh), GITHUB_REPOSITORY

NAME="$1"
TAG="$2"
PRE_JSON="$3"
POST_JSON="$4"

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

OWNER="${GITHUB_REPOSITORY%/*}"
REPO="${GITHUB_REPOSITORY#*/}"
BRANCH="reports"

push_file() {
  local remote_path="$1"
  local local_file="$2"
  local message="$3"

  local content
  content=$(base64 < "$local_file" | tr -d '\n')

  # Get current file SHA (required for updates, absent for creates).
  local sha=""
  if sha=$(gh api "repos/${OWNER}/${REPO}/contents/${remote_path}?ref=${BRANCH}" \
                   --jq '.sha' 2>/dev/null); then
    true
  else
    sha=""
  fi

  if [ -n "$sha" ]; then
    gh api --method PUT "repos/${OWNER}/${REPO}/contents/${remote_path}" \
      --field "message=${message}" \
      --field "content=${content}" \
      --field "sha=${sha}" \
      --field "branch=${BRANCH}" \
      --silent
  else
    gh api --method PUT "repos/${OWNER}/${REPO}/contents/${remote_path}" \
      --field "message=${message}" \
      --field "content=${content}" \
      --field "branch=${BRANCH}" \
      --silent
  fi

  echo "✓ Pushed ${remote_path} to ${BRANCH} branch"
}

push_file "reports/${NAME}/${TAG}/pre.json"  "$PRE_JSON"  "chore: update ${NAME}/${TAG} pre-patch report"
push_file "reports/${NAME}/${TAG}/post.json" "$POST_JSON" "chore: update ${NAME}/${TAG} post-patch report"
