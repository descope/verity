#!/usr/bin/env bash
set -euo pipefail

# Collects all image data from Copa discovery results and generates vulnerability catalog
# Usage: generate-catalog.sh <results-json> <reports-dir> <registry> <output-file>

RESULTS_JSON="${1:-results.json}"
REPORTS_DIR="${2:-reports}"
REGISTRY="${3:-}"
OUTPUT_FILE="${4:-site/src/data/catalog.json}"

if [ -z "$REGISTRY" ]; then
  echo "Error: REGISTRY is required"
  exit 1
fi

echo "Building images list from $RESULTS_JSON..."

# Build images.json from results.json (all WouldPatch + Skipped images)
# Schema: {original, patched, report}
images_json=$(jq -c '[
  .[] | select(.status == "WouldPatch" or .status == "Skipped") |
  {
    original: .source,
    patched: .target,
    report: (.source | gsub("[/:]"; "_") + ".json")
  }
]' "$RESULTS_JSON")

mkdir -p .verity
echo "$images_json" > .verity/images.json

IMAGE_COUNT=$(echo "$images_json" | jq 'length')
echo "✓ Collected $IMAGE_COUNT image(s) for catalog"

echo "Generating catalog..."
./verity catalog \
  --images-json .verity/images.json \
  --reports-dir "$REPORTS_DIR" \
  --registry "$REGISTRY" \
  --output "$OUTPUT_FILE"

echo "✓ Catalog generated at $OUTPUT_FILE"
