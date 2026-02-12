#!/bin/bash
set -euo pipefail

CHARTS_DIR="${1:-.}"

echo "=== Validating Wrapper Charts ==="

found_charts=false
for chart_yaml in "${CHARTS_DIR}"/charts/*/Chart.yaml; do
  if [ ! -f "$chart_yaml" ]; then
    continue
  fi

  found_charts=true
  chart_dir=$(dirname "$chart_yaml")
  chart_name=$(basename "$chart_dir")
  echo ""
  echo "Validating ${chart_name}..."

  # Check required files exist
  for file in Chart.yaml values.yaml .helmignore; do
    if [ ! -f "${chart_dir}/${file}" ]; then
      echo "Missing ${file} in ${chart_name}"
      exit 1
    fi
  done

  # Validate Chart.yaml structure
  if ! yq eval '.apiVersion' "${chart_yaml}" | grep -q "v2"; then
    echo "Invalid apiVersion in ${chart_name}/Chart.yaml"
    exit 1
  fi

  if ! yq eval '.dependencies | length' "${chart_yaml}" | grep -q "1"; then
    echo "Missing or invalid dependencies in ${chart_name}/Chart.yaml"
    exit 1
  fi

  # Lint the chart
  if ! helm lint "${chart_dir}"; then
    echo "Helm lint failed for ${chart_name}"
    exit 1
  fi

  echo "${chart_name} is valid"
done

if [ "$found_charts" = false ]; then
  echo "No wrapper charts found in ${CHARTS_DIR}/charts/"
  exit 0
fi

echo ""
echo "All wrapper charts validated successfully"
