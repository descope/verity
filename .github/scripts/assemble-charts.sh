#!/bin/bash
# Assemble and optionally publish Helm charts
set -euo pipefail

MANIFEST="${1:-.verity/manifest.json}"
RESULTS_DIR="${2:-.verity/results}"
REPORTS_DIR="${3:-.verity/reports}"
OUTPUT_DIR="${4:-.verity/charts}"
REGISTRY="${5:-}"
PUBLISH="${6:-false}"

# Collect results and reports
mkdir -p "$RESULTS_DIR" "$REPORTS_DIR"
for d in .verity/artifacts/patch-result-*; do
  [ -d "$d" ] || continue
  find "$d" -name '*.json' -path '*/results/*' -exec cp -n {} "$RESULTS_DIR/" \; 2>/dev/null || true
  find "$d" -name '*.json' -path '*/reports/*' -exec cp -n {} "$REPORTS_DIR/" \; 2>/dev/null || true
done

# Run assemble
publish_flag=""
if [ "$PUBLISH" = "true" ]; then
  publish_flag="--publish"
fi

./verity assemble \
  --manifest "$MANIFEST" \
  --results-dir "$RESULTS_DIR" \
  --reports-dir "$REPORTS_DIR" \
  --output-dir "$OUTPUT_DIR" \
  --registry "$REGISTRY" \
  $publish_flag

# Build matrix from published-charts.json
published="$OUTPUT_DIR/published-charts.json"
if [ -f "$published" ] && [ "$(jq length "$published")" -gt 0 ]; then
  matrix=$(jq -c '{include: .}' "$published")
  echo "chart_matrix=$matrix" >> "$GITHUB_OUTPUT"
  echo "has_charts=true" >> "$GITHUB_OUTPUT"
else
  echo "has_charts=false" >> "$GITHUB_OUTPUT"
fi
