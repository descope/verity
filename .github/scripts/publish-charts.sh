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

if [ ! -d "${CHARTS_DIR}/charts" ]; then
  echo "No charts directory found at ${CHARTS_DIR}/charts"
  exit 0
fi

echo "=== Publishing Wrapper Charts ==="
echo "Registry: ${REGISTRY}/${ORG}/charts"
echo ""

published=0
for chart_yaml in "${CHARTS_DIR}"/charts/*/Chart.yaml; do
  if [ ! -f "$chart_yaml" ]; then
    continue
  fi

  chart_dir=$(dirname "$chart_yaml")
  chart_name=$(basename "$chart_dir")
  echo "Publishing ${chart_name}..."

  # Build dependencies
  echo "  Building dependencies..."
  helm dependency build "${chart_dir}"

  # Package chart
  echo "  Packaging chart..."
  helm package "${chart_dir}" --destination /tmp/helm-packages

  # Get chart version
  version=$(yq eval '.version' "${chart_yaml}")

  # Push to registry
  echo "  Pushing to ${REGISTRY}/${ORG}/charts/${chart_name}:${version}..."
  helm push "/tmp/helm-packages/${chart_name}-${version}.tgz" "oci://${REGISTRY}/${ORG}/charts"

  echo "Published ${chart_name}:${version}"
  echo ""
  published=$((published + 1))
done

if [ $published -eq 0 ]; then
  echo "No wrapper charts found to publish"
else
  echo "Successfully published ${published} chart(s)"
fi
