#!/usr/bin/env bash
set -euo pipefail

# Generates per-image manifest report JSON files from Copa patch results.
# Required env vars: STAGING_REGISTRY, IS_PR

: "${STAGING_REGISTRY:?STAGING_REGISTRY is required}"
: "${IS_PR:?IS_PR is required}"

mkdir -p manifest-results

# Record Copa's computed target tag for each image that was successfully manifested
jq -r '[.[] | {image, target}] | unique_by(.image) | .[] | @base64' all-patches.json | \
while IFS= read -r row; do
  entry=$(echo "$row" | base64 -d)
  IMAGE=$(echo "$entry" | jq -r '.image')
  COPA_TARGET=$(echo "$entry" | jq -r '.target')
  if [ "$IS_PR" = "true" ]; then
    STAGING_TAG="${COPA_TARGET##*/}"
    MANIFEST="${STAGING_REGISTRY}:${STAGING_TAG//:/-}"
  else
    MANIFEST="${COPA_TARGET}"
  fi
  SAFE_IMAGE_FILE=$(echo "$IMAGE" | tr '/: ' '___')
  echo "{\"image\": \"$IMAGE\", \"manifest\": \"$MANIFEST\"}" \
    > "manifest-results/${SAFE_IMAGE_FILE}.json"
done
