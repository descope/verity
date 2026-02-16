#!/bin/bash
# Sign container image with cosign
set -euo pipefail

IMAGE="$1"
DIGEST="$2"
TOKEN="${3:-}"

if [ -n "$TOKEN" ]; then
  echo "$TOKEN" | cosign login ghcr.io -u "${GITHUB_ACTOR:-github-actions}" --password-stdin
fi

cosign sign --yes "${IMAGE}@${DIGEST}"
echo "✓ Signed ${IMAGE}@${DIGEST}"
