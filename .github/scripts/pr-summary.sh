#!/usr/bin/env bash
set -euo pipefail

# Generates PR testing summary for GitHub Actions
# Usage: pr-summary.sh <registry> <images-json> <catalog-json>

REGISTRY="${1:-}"
IMAGES_JSON="${2:-.verity/images.json}"
CATALOG_JSON="${3:-site/src/data/catalog.json}"

if [ -z "$REGISTRY" ]; then
  echo "Error: REGISTRY is required"
  exit 1
fi

IMAGE_COUNT=$(jq 'length' "$IMAGES_JSON")

cat >> "$GITHUB_STEP_SUMMARY" <<EOF
### PR Testing Summary

✅ Copa matrix pipeline tested successfully

**Test Registry:** \`$REGISTRY\`
**Images in Catalog:** \`$IMAGE_COUNT\`

**Pipeline Steps Validated:**
- ✅ Trivy scanning (parallel with server)
- ✅ Copa dry-run discovery
- ✅ Matrix parallel patching
- ✅ Catalog generation
- ✅ Site build

**Catalog Stats:**
EOF

jq -r '"- Total Images: \(.summary.totalImages)\n- Total Vulnerabilities: \(.summary.totalVulns)\n- Fixable: \(.summary.fixableVulns)"' \
  "$CATALOG_JSON" >> "$GITHUB_STEP_SUMMARY"

echo "✓ PR summary generated"
