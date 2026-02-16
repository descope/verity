#!/bin/bash
# Sign Helm chart with cosign
set -euo pipefail

OCI_NAME="$1"
DIGEST="$2"
TOKEN="${3:-}"

if [ -n "$TOKEN" ]; then
  echo "$TOKEN" | cosign login ghcr.io -u "$(echo "$GITHUB_ACTOR" || echo "github-actions")" --password-stdin
fi

cosign sign --yes "${OCI_NAME}@${DIGEST}"
echo "✓ Signed chart ${OCI_NAME}@${DIGEST}"
