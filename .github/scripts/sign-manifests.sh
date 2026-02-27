#!/usr/bin/env bash
set -euo pipefail

# Signs all GHCR manifest lists with cosign keyless signing.
# Reads manifest references from ghcr-manifests.txt.

if [ ! -f ghcr-manifests.txt ]; then
  echo "No manifests to sign"
  exit 0
fi

while IFS= read -r manifest; do
  cosign sign --yes "$manifest"
  echo "✓ Signed: $manifest"
done < ghcr-manifests.txt
