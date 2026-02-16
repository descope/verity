#!/bin/bash
# Discover images from values.yaml and Chart.yaml, output matrix for GitHub Actions
set -euo pipefail

IMAGES_FILE="${1:-values.yaml}"
CHART_FILE="${2:-Chart.yaml}"
DISCOVER_DIR="${3:-.verity}"

echo "Discovering images from $IMAGES_FILE and $CHART_FILE"

./verity discover \
  --images "$IMAGES_FILE" \
  --chart-file "$CHART_FILE" \
  --discover-dir "$DISCOVER_DIR"

matrix=$(cat "$DISCOVER_DIR/matrix.json")
echo "matrix=$matrix" >> "$GITHUB_OUTPUT"

count=$(echo "$matrix" | jq '.include | length')
if [ "$count" -gt 0 ]; then
  echo "has_images=true" >> "$GITHUB_OUTPUT"
else
  echo "has_images=false" >> "$GITHUB_OUTPUT"
fi

echo "Found $count unique images to patch"
