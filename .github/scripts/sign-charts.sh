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

echo "=== Signing Wrapper Charts ==="
echo "Registry: ${REGISTRY}/${ORG}/charts"
echo ""

signed=0
for chart_yaml in "${CHARTS_DIR}"/charts/*/Chart.yaml; do
  if [ ! -f "$chart_yaml" ]; then
    continue
  fi

  chart_dir=$(dirname "$chart_yaml")
  chart_name=$(basename "$chart_dir")
  version=$(yq eval '.version' "${chart_yaml}")
  oci_ref="${REGISTRY}/${ORG}/charts/${chart_name}"

  echo "Signing ${chart_name}:${version}..."

  # Resolve the OCI artifact digest
  digest=$(crane digest "${oci_ref}:${version}" 2>/dev/null || true)
  if [ -z "$digest" ]; then
    echo "  WARNING: Could not resolve digest for ${oci_ref}:${version}, skipping"
    continue
  fi

  echo "  Digest: ${digest}"

  # Sign with cosign (keyless via OIDC)
  echo "  Signing with cosign..."
  cosign sign --yes "${oci_ref}@${digest}"

  echo "  Signed ${chart_name}:${version}"
  echo ""
  signed=$((signed + 1))
done

if [ $signed -eq 0 ]; then
  echo "No charts found to sign"
else
  echo "Successfully signed ${signed} chart(s)"
fi
