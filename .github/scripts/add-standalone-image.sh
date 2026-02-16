#!/bin/bash
set -euo pipefail

# Adds a standalone image entry to charts/standalone/values.yaml and creates a PR.
# Expects environment variables: IMAGE_NAME, IMAGE_REPOSITORY, IMAGE_TAG, IMAGE_REGISTRY, ISSUE_NUMBER

: "${IMAGE_NAME:?IMAGE_NAME is required}"
: "${IMAGE_REPOSITORY:?IMAGE_REPOSITORY is required}"
: "${IMAGE_TAG:?IMAGE_TAG is required}"
: "${IMAGE_REGISTRY:?IMAGE_REGISTRY is required}"
: "${ISSUE_NUMBER:?ISSUE_NUMBER is required}"

STANDALONE_VALUES="charts/standalone/values.yaml"

# Check for duplicate
export IMAGE_NAME IMAGE_REPOSITORY IMAGE_TAG IMAGE_REGISTRY
if yq e '.[strenv(IMAGE_NAME)]' "$STANDALONE_VALUES" 2>/dev/null | grep -q repository; then
  echo "Image ${IMAGE_NAME} already exists in ${STANDALONE_VALUES}"
  gh issue comment "${ISSUE_NUMBER}" \
    --body "Image **${IMAGE_NAME}** already exists in standalone images. Closing as duplicate."
  gh issue close "${ISSUE_NUMBER}"
  exit 0
fi

# Add image entry using env vars to avoid injection
yq e '.[strenv(IMAGE_NAME)].image.registry = strenv(IMAGE_REGISTRY) |
      .[strenv(IMAGE_NAME)].image.repository = strenv(IMAGE_REPOSITORY) |
      .[strenv(IMAGE_NAME)].image.tag = strenv(IMAGE_TAG)' -i "$STANDALONE_VALUES"

# Sanitize branch name
SAFE_NAME=$(echo "${IMAGE_NAME}" | tr -cs '[:alnum:]-' '-' | sed 's/^-//;s/-$//')
BRANCH="add-image/${SAFE_NAME}"

git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"
git checkout -b "${BRANCH}"
git add "$STANDALONE_VALUES"
git commit -m "feat: add ${IMAGE_NAME} standalone image

Adds ${IMAGE_REGISTRY}/${IMAGE_REPOSITORY}:${IMAGE_TAG} to standalone images.

The update-images workflow will pick this up and it will be patched
by the scan-and-patch workflow.

Closes #${ISSUE_NUMBER}"

git push -u origin "${BRANCH}"

gh pr create \
  --title "feat: add ${IMAGE_NAME} standalone image" \
  --body "Adds standalone image \`${IMAGE_REGISTRY}/${IMAGE_REPOSITORY}:${IMAGE_TAG}\`.

## What happens next

1. This image is added to the standalone chart (\`charts/standalone/\`)
2. **update-images workflow** will scan and include it in the full image list
3. **scan-and-patch workflow** will patch and publish it to GHCR

Closes #${ISSUE_NUMBER}" \
  --label new-image
