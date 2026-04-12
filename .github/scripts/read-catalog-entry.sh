#!/usr/bin/env bash
set -euo pipefail

# Reads image URL and goVcsUrl from copa-config.yaml for a given image name + tag.
# Inputs: IMAGE_NAME, IMAGE_TAG (env vars)
# Outputs: source, go_vcs_url (via GITHUB_OUTPUT)

: "${IMAGE_NAME:?IMAGE_NAME is required}"
: "${IMAGE_TAG:?IMAGE_TAG is required}"

IMAGE=$(yq -r ".images[] | select(.name == \"${IMAGE_NAME}\") | .image" copa-config.yaml)
VCS_URL=$(yq -r ".images[] | select(.name == \"${IMAGE_NAME}\") | .goVcsUrl // \"\"" copa-config.yaml)
VCS_PREFIX=$(yq -r ".images[] | select(.name == \"${IMAGE_NAME}\") | .goVcsTagPrefix // \"\"" copa-config.yaml)

if [ -n "$VCS_URL" ]; then
  VCS_URL="${VCS_URL}@${VCS_PREFIX}${IMAGE_TAG}"
else
  VCS_URL=""
fi

SOURCE="${IMAGE}:${IMAGE_TAG}"
echo "source=${SOURCE}" >> "$GITHUB_OUTPUT"
echo "go_vcs_url=${VCS_URL}" >> "$GITHUB_OUTPUT"
echo "Catalog: image=${IMAGE} tag=${IMAGE_TAG} goVcsUrl=${VCS_URL}"
