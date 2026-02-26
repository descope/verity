#!/usr/bin/env bash
set -euo pipefail

# Runs Copa dry-run discovery and generates matrix JSON for GitHub Actions
# Usage: copa-discover.sh <config-file> <reports-dir> <output-file>

CONFIG_FILE="${1:-copa-config.yaml}"
REPORTS_DIR="${2:-reports}"
OUTPUT_FILE="${3:-$GITHUB_OUTPUT}"

echo "Running Copa discovery..."
COPA_EXPERIMENTAL=1 copa patch \
  --config "$CONFIG_FILE" \
  --report "$REPORTS_DIR" \
  --pkg-types os,library \
  --library-patch-level minor \
  --dry-run \
  --output-json results.json

# Filter to only images that need patching and expand with platforms
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"

# Create platform-expanded matrix
jq -c --arg platforms "$PLATFORMS" '[
  .[] | select(.status == "WouldPatch") |
  . as $img |
  ($platforms | split(",")) | .[] |
  # Extract tag from source image (e.g., nginx:1.29.3 -> 1.29.3)
  ($img.source | split(":")[1] // "latest") as $tag |
  # Sanitize tag for job name (replace dots/colons with dashes)
  ($tag | gsub("[.:]"; "-")) as $safe_tag |
  {
    name: (($img.name + "-" + $safe_tag + "-" + (. | split("/")[1])) | gsub("/"; "-")),
    image: $img.name,
    source: $img.source,
    target: $img.target,
    platform: .,
    runner: (if . == "linux/arm64" then "ubuntu-24.04-arm" else "ubuntu-24.04" end)
  }
]' results.json > matrix.json

count=$(jq 'length' matrix.json)
if [ "$count" -gt 0 ]; then
  echo "has_images=true" >> "$OUTPUT_FILE"
  echo "matrix={\"include\":$(cat matrix.json)}" >> "$OUTPUT_FILE"
  echo "✓ Found $count platform/image combination(s) that need patching:"
  jq -r '.[] | "  - \(.name): \(.source) (\(.platform))"' matrix.json
else
  echo "has_images=false" >> "$OUTPUT_FILE"
  echo "✓ No images need patching (all up to date)"
fi

# Check if any images should appear in the catalog (WouldPatch or Skipped)
catalog_count=$(jq '[.[] | select(.status == "WouldPatch" or .status == "Skipped")] | length' results.json)
if [ "$catalog_count" -gt 0 ]; then
  echo "has_catalog_images=true" >> "$OUTPUT_FILE"
else
  echo "has_catalog_images=false" >> "$OUTPUT_FILE"
fi
