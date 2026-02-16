#!/bin/bash
# Get digest of patched image from result JSON
set -euo pipefail

RESULTS_DIR="${1:-.verity/results}"

result_file=$(find "$RESULTS_DIR" -name '*.json' -print -quit 2>/dev/null)
if [ -z "$result_file" ]; then
  echo "has_image=false" >> "$GITHUB_OUTPUT"
  exit 0
fi

registry=$(jq -r '.patched_registry // empty' "$result_file")
repo=$(jq -r '.patched_repository // empty' "$result_file")
tag=$(jq -r '.patched_tag // empty' "$result_file")
err=$(jq -r '.error // empty' "$result_file")

if [ -z "$registry" ] || [ -z "$repo" ] || [ -z "$tag" ]; then
  echo "has_image=false" >> "$GITHUB_OUTPUT"
  exit 0
fi
if [ -n "$err" ]; then
  echo "has_image=false" >> "$GITHUB_OUTPUT"
  exit 0
fi

image="${registry}/${repo}"
image_with_tag="${image}:${tag}"
digest=$(crane digest "$image_with_tag")

{
  echo "image=${image}"
  echo "image_with_tag=${image_with_tag}"
  echo "digest=${digest}"
  echo "has_image=true"
} >> "$GITHUB_OUTPUT"

echo "Resolved ${image_with_tag} → ${digest}"
