#!/bin/bash
set -euo pipefail

# Adds an image entry to copa-config.yaml and creates a PR.
# Expects environment variables: IMAGE_NAME, IMAGE_REPOSITORY, IMAGE_TAG, IMAGE_REGISTRY, ISSUE_NUMBER

: "${IMAGE_NAME:?IMAGE_NAME is required}"
: "${IMAGE_REPOSITORY:?IMAGE_REPOSITORY is required}"
: "${IMAGE_TAG:?IMAGE_TAG is required}"
: "${IMAGE_REGISTRY:?IMAGE_REGISTRY is required}"
: "${ISSUE_NUMBER:?ISSUE_NUMBER is required}"

COPA_CONFIG="copa-config.yaml"

# Check for duplicate
export IMAGE_NAME
if yq e '.images[] | select(.name == strenv(IMAGE_NAME)) | .name' "$COPA_CONFIG" 2>/dev/null | grep -q .; then
  echo "Image ${IMAGE_NAME} already exists in ${COPA_CONFIG}"
  gh issue comment "${ISSUE_NUMBER}" \
    --body "Image **${IMAGE_NAME}** already exists in copa-config.yaml. Closing as duplicate."
  gh issue close "${ISSUE_NUMBER}"
  exit 0
fi

# Build full image reference and add entry to copa-config.yaml using env vars to avoid injection
IMAGE_REF="${IMAGE_REGISTRY}/${IMAGE_REPOSITORY}"
export IMAGE_REF
yq e '.images += [{"name": strenv(IMAGE_NAME), "image": strenv(IMAGE_REF), "platforms": ["linux/amd64", "linux/arm64"], "tags": {"strategy": "pattern", "pattern": "^\\d+\\.\\d+\\.\\d+$", "maxTags": 3}}]' -i "$COPA_CONFIG"

# Sanitize branch name
SAFE_NAME=$(echo "${IMAGE_NAME}" | tr -cs '[:alnum:]-' '-' | sed 's/^-//;s/-$//')
BRANCH="add-image/${SAFE_NAME}"

git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"
git checkout -b "${BRANCH}"
git add "$COPA_CONFIG"
git commit -m "feat: add ${IMAGE_NAME} image

Adds ${IMAGE_REGISTRY}/${IMAGE_REPOSITORY}:${IMAGE_TAG} to copa-config.yaml.

Copa will patch this image on the next scan-and-patch workflow run.

Closes #${ISSUE_NUMBER}"

git push -u origin "${BRANCH}"

gh pr create \
  --title "feat: add ${IMAGE_NAME} image" \
  --body "Adds image \`${IMAGE_REGISTRY}/${IMAGE_REPOSITORY}:${IMAGE_TAG}\` to \`copa-config.yaml\`.

## What happens next

1. This image is added to \`copa-config.yaml\` under \`images:\`
2. **scan-and-patch workflow** will patch and publish it to GHCR

Closes #${ISSUE_NUMBER}" \
  --label new-image
