#!/bin/bash
# Generate SBOM for container image using Trivy
set -euo pipefail

IMAGE="$1"
DIGEST="$2"
OUTPUT_FILE="${3:-.verity/sbom.cdx.json}"

trivy image --format cyclonedx --output "$OUTPUT_FILE" "${IMAGE}@${DIGEST}"
echo "✓ Generated SBOM: $OUTPUT_FILE"
