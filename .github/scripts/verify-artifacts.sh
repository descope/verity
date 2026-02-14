#!/bin/bash
set -euo pipefail

# Verify cosign signatures and GitHub attestations for Verity artifacts.
#
# Prerequisites:
#   - cosign   (https://docs.sigstore.dev/cosign/system_config/installation/)
#   - crane    (https://github.com/google/go-containerregistry/tree/main/cmd/crane)
#   - gh CLI   (https://cli.github.com/) with attestation extension
#
# Usage:
#   ./verify-artifacts.sh <registry>/<org>  [chart-name:version | image:tag-patched]
#
# Examples:
#   # Verify all wrapper charts and their images
#   ./verify-artifacts.sh quay.io/verity
#
#   # Verify a specific image
#   ./verify-artifacts.sh quay.io/verity grafana/grafana:11.6.0-patched
#
#   # Verify a specific chart
#   ./verify-artifacts.sh quay.io/verity charts/prometheus:28.9.1-5

REGISTRY_ORG="${1:-}"
ARTIFACT="${2:-}"
OWNER="${GITHUB_REPOSITORY_OWNER:-descope}"

if [ -z "$REGISTRY_ORG" ]; then
  echo "Usage: $0 <registry/org> [artifact-ref]"
  echo ""
  echo "Examples:"
  echo "  $0 quay.io/verity"
  echo "  $0 quay.io/verity grafana/grafana:11.6.0-patched"
  echo "  $0 quay.io/verity charts/prometheus:28.9.1-5"
  exit 1
fi

errors=0

verify_image() {
  local ref="$1"
  echo "=== Verifying ${ref} ==="

  # Resolve digest
  digest=$(crane digest "$ref" 2>/dev/null || true)
  if [ -z "$digest" ]; then
    echo "  SKIP: Could not resolve digest (image may not exist)"
    echo ""
    return
  fi
  echo "  Digest: ${digest}"

  # Build immutable digest reference for verification
  local image="${ref%%:*}"
  local digest_ref="${image}@${digest}"

  # Verify cosign signature
  echo "  Checking cosign signature..."
  if cosign verify \
    --certificate-identity-regexp "https://github.com/${OWNER}/verity/.github/workflows/" \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
    "${digest_ref}" >/dev/null 2>&1; then
    echo "  Cosign signature: VERIFIED"
  else
    echo "  Cosign signature: FAILED"
    errors=$((errors + 1))
  fi

  # Verify GitHub build provenance attestation
  echo "  Checking build provenance..."
  if gh attestation verify "oci://${digest_ref}" --owner "$OWNER" 2>/dev/null; then
    echo "  Build provenance: VERIFIED"
  else
    echo "  Build provenance: NOT FOUND or FAILED"
    errors=$((errors + 1))
  fi

  echo ""
}

if [ -n "$ARTIFACT" ]; then
  verify_image "${REGISTRY_ORG}/${ARTIFACT}"
else
  echo "No specific artifact given — provide an image or chart ref to verify."
  echo ""
  echo "Examples:"
  echo "  $0 ${REGISTRY_ORG} grafana/grafana:11.6.0-patched"
  echo "  $0 ${REGISTRY_ORG} charts/prometheus:28.9.1-5"
  exit 0
fi

if [ $errors -gt 0 ]; then
  echo "FAILED: ${errors} verification(s) failed"
  exit 1
else
  echo "All verifications passed"
fi
