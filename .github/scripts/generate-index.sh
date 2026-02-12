#!/bin/bash
set -euo pipefail

CHARTS_DIR="${1:-.}"
OUTPUT_FILE="${2:-/tmp/index.md}"
REGISTRY="${3:-ghcr.io}"
ORG="${4}"

if [ -z "$ORG" ]; then
  echo "Usage: $0 <charts-dir> <output-file> <registry> <org>"
  echo "Example: $0 ./charts /tmp/index.md ghcr.io myorg"
  exit 1
fi

echo "=== Generating Chart Index ==="

cat > "$OUTPUT_FILE" << 'EOF'
# Verity Patched Charts

Self-maintained registry of security-patched Helm charts.

## Available Charts

EOF

found_charts=false
for chart_yaml in "${CHARTS_DIR}"/charts/*/Chart.yaml; do
  if [ ! -f "$chart_yaml" ]; then
    continue
  fi

  found_charts=true
  chart_dir=$(dirname "$chart_yaml")
  chart_name=$(basename "$chart_dir")
  version=$(yq eval '.version' "${chart_yaml}")
  description=$(yq eval '.description' "${chart_yaml}")

  cat >> "$OUTPUT_FILE" << EOF
### ${chart_name}

${description}

\`\`\`bash
helm install my-release oci://${REGISTRY}/${ORG}/charts/${chart_name} --version ${version}
\`\`\`

EOF
done

if [ "$found_charts" = false ]; then
  echo "No charts found" >> "$OUTPUT_FILE"
  echo "No wrapper charts found"
else
  echo "Chart index generated: $OUTPUT_FILE"
  cat "$OUTPUT_FILE"
fi
