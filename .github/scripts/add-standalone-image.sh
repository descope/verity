#!/bin/bash
set -euo pipefail

# Adds a standalone image entry to values.yaml and creates a PR.
# Expects environment variables: IMAGE_NAME, IMAGE_REPOSITORY, IMAGE_TAG, IMAGE_REGISTRY, ISSUE_NUMBER

: "${IMAGE_NAME:?IMAGE_NAME is required}"
: "${IMAGE_REPOSITORY:?IMAGE_REPOSITORY is required}"
: "${IMAGE_TAG:?IMAGE_TAG is required}"
: "${IMAGE_REGISTRY:?IMAGE_REGISTRY is required}"
: "${ISSUE_NUMBER:?ISSUE_NUMBER is required}"

# Check for duplicate
if yq e ".\"${IMAGE_NAME}\"" values.yaml 2>/dev/null | grep -q repository; then
  echo "Image ${IMAGE_NAME} already exists in values.yaml"
  gh issue comment "${ISSUE_NUMBER}" \
    --body "Image **${IMAGE_NAME}** already exists in values.yaml. Closing as duplicate."
  gh issue close "${ISSUE_NUMBER}"
  exit 0
fi

# Add image entry using env vars to avoid injection
export IMAGE_NAME IMAGE_REPOSITORY IMAGE_TAG IMAGE_REGISTRY
yq e '.[strenv(IMAGE_NAME)].image.registry = strenv(IMAGE_REGISTRY) |
      .[strenv(IMAGE_NAME)].image.repository = strenv(IMAGE_REPOSITORY) |
      .[strenv(IMAGE_NAME)].image.tag = strenv(IMAGE_TAG)' -i values.yaml

# Sanitize branch name
SAFE_NAME=$(echo "${IMAGE_NAME}" | tr -cs '[:alnum:]-' '-' | sed 's/^-//;s/-$//')
BRANCH="add-image/${SAFE_NAME}"

git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"
git checkout -b "${BRANCH}"
git add values.yaml
git commit -m "feat: add ${IMAGE_NAME} standalone image"
git push -u origin "${BRANCH}"
gh pr create \
  --title "Add ${IMAGE_NAME} image" \
  --body "Adds standalone image ${IMAGE_REGISTRY}/${IMAGE_REPOSITORY}:${IMAGE_TAG}.

Closes #${ISSUE_NUMBER}" \
  --label new-image
