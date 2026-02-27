#!/usr/bin/env bash
set -euo pipefail

# Scans images discovered by Copa dry-run that are missing Trivy reports.
# verity scan only processes the images: section; chart-discovered images need separate scanning.
# Usage: scan-chart-images.sh <results-json> <reports-dir>

RESULTS_JSON="${1:-results.json}"
REPORTS_DIR="${2:-reports}"

jq -r '.[] | select(.status == "WouldPatch") | .source' "$RESULTS_JSON" | \
while IFS= read -r source; do
  report="${REPORTS_DIR}/$(echo "$source" | sed 's/[\/:]/_/g').json"
  if [ ! -f "$report" ]; then
    echo "Scanning chart image: $source"
    if trivy image \
      --server http://localhost:4954 \
      --vuln-type os,library \
      --format json \
      --quiet \
      "$source" > "$report"; then
      echo "✓ Scanned: $source"
    else
      echo "⚠️  Failed to scan $source, creating empty report"
      echo "{\"ArtifactName\": \"$source\", \"Results\": []}" > "$report"
    fi
  fi
done
