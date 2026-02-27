#!/usr/bin/env bash
set -euo pipefail

# Builds a GitHub Actions matrix JSON for the attest job.
# Reads ghcr-manifests.txt, resolves digests via crane, writes to GITHUB_OUTPUT.

MATRIX_ITEMS="[]"
if [ -f ghcr-manifests.txt ]; then
  while IFS= read -r manifest; do
    DIGEST=$(crane digest "$manifest")
    IMAGE="${manifest%:*}"
    MATRIX_ITEMS=$(echo "$MATRIX_ITEMS" | jq -c \
      --arg image "$IMAGE" --arg digest "$DIGEST" \
      '. += [{"image": $image, "digest": $digest}]')
  done < ghcr-manifests.txt
fi

COUNT=$(echo "$MATRIX_ITEMS" | jq 'length')
if [ "$COUNT" -gt 0 ]; then
  echo "has_manifests=true" >> "$GITHUB_OUTPUT"
  echo "attestation_matrix={\"include\":$MATRIX_ITEMS}" >> "$GITHUB_OUTPUT"
else
  echo "has_manifests=false" >> "$GITHUB_OUTPUT"
fi
