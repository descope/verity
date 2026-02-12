#!/bin/bash
set -euo pipefail

CHARTS_DIR="${1:-.}"
REGISTRY="${2:-ghcr.io}"
ORG="${3}"

if [ -z "$ORG" ]; then
  echo "Usage: $0 <charts-dir> <registry> <org>"
  echo "Example: $0 ./charts ghcr.io myorg"
  exit 1
fi

echo "=== Verifying Patched Images ==="
echo "Registry: ${REGISTRY}/${ORG}"
echo ""

for chart_yaml in "${CHARTS_DIR}"/charts/*/Chart.yaml; do
  if [ ! -f "$chart_yaml" ]; then
    continue
  fi

  chart_dir=$(dirname "$chart_yaml")
  chart_name=$(basename "$chart_dir")
  values_file="${chart_dir}/values.yaml"

  if [ ! -f "${values_file}" ]; then
    echo "No values.yaml found for ${chart_name}, skipping"
    continue
  fi

  echo "Checking images from ${chart_name}..."

  # Extract image references from values.yaml (values are namespaced under chart name)
  yq eval ".[\"${chart_name}\"]" "${values_file}" -o json 2>/dev/null | \
  jq -r '.. | objects | select(has("registry") and has("repository") and has("tag")) |
         "\(.registry)/\(.repository):\(.tag)"' | \
  while read -r image; do
    if [ -n "$image" ] && [[ "$image" == "${REGISTRY}/${ORG}"/* ]]; then
      echo -n "  ${image} ... "

      # Check if image exists
      if docker manifest inspect "${image}" >/dev/null 2>&1; then
        echo "OK"
      else
        echo "NOT FOUND"
      fi
    fi
  done

  echo ""
done

echo "Image verification complete"
