#!/usr/bin/env bash
set -euo pipefail

# Creates multi-platform manifest lists from platform-specific staging images.
# In PRs, pushes to staging registry to avoid polluting production.
# Required env vars: STAGING_REGISTRY, IS_PR

: "${STAGING_REGISTRY:?STAGING_REGISTRY is required}"
: "${IS_PR:?IS_PR is required}"

# Collect all patched images
# shellcheck disable=SC2024
cat patch-results/*.json | jq -s '.' > all-patches.json

# Get unique image:tag combinations (group by image name and source tag)
IMAGES=$(jq -r '.[] | .image + ":" + (.source | split(":")[1])' all-patches.json | sort -u)

for IMAGE_TAG in $IMAGES; do
  IMAGE=$(echo "$IMAGE_TAG" | cut -d: -f1)
  TAG=$(echo "$IMAGE_TAG" | cut -d: -f2-)

  echo "Creating manifest list for $IMAGE:$TAG..."

  # Build list of platform-specific source tags from GHCR staging.
  # Sanitize image name to match how PLATFORM_TAG was constructed (slashes → dashes).
  SOURCE_TAGS=$(jq -r --arg img "$IMAGE" --arg tag "$TAG" \
    --arg registry "$STAGING_REGISTRY" \
    'map(select(.image == $img and (.source | split(":")[1]) == $tag)) |
     .[] |
     (.platform | split("/")[1]) as $arch |
     ($img | gsub("[/: ]"; "-")) as $safe_img |
     $registry + ":" + $safe_img + "-" + $tag + "-patched-" + $arch' \
    all-patches.json)

  # Verify all platform images exist before creating manifest
  ALL_EXIST=true
  for source_tag in $SOURCE_TAGS; do
    if ! crane digest "$source_tag" > /dev/null 2>&1; then
      echo "⚠️  Skipping $IMAGE:$TAG - missing platform image: $source_tag"
      ALL_EXIST=false
      break
    fi
  done

  if [ "$ALL_EXIST" = "false" ]; then
    continue
  fi

  # Use Copa's computed target tag (may include a version suffix, e.g. 2.5.0-patched-1)
  COPA_TARGET=$(jq -r --arg img "$IMAGE" --arg tag "$TAG" \
    'map(select(.image == $img and (.source | split(":")[1]) == $tag)) | first | .target' \
    all-patches.json)

  if [ -z "$COPA_TARGET" ] || [ "$COPA_TARGET" = "null" ]; then
    echo "⚠️  Skipping $IMAGE:$TAG - no target manifest tag found in all-patches.json"
    continue
  fi

  if [ "$IS_PR" = "true" ]; then
    # Replace ':' in tag so it's OCI-valid (e.g. k8s-sidecar:2.3.0-patched-1 → k8s-sidecar-2.3.0-patched-1)
    STAGING_TAG="${COPA_TARGET##*/}"
    MANIFEST_TAG="${STAGING_REGISTRY}:${STAGING_TAG//:/-}"
  else
    MANIFEST_TAG="$COPA_TARGET"
  fi

  # imagetools copies platform images and creates the manifest list in one step
  # shellcheck disable=SC2086
  docker buildx imagetools create --tag "$MANIFEST_TAG" $SOURCE_TAGS
  echo "✓ Created manifest: $MANIFEST_TAG"
  echo "$MANIFEST_TAG" >> ghcr-manifests.txt
done
