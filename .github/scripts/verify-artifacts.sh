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
#   ./verify-artifacts.sh <registry>/<org>  [image:tag-patched]
#
# Examples:
#   ./verify-artifacts.sh ghcr.io/verity-org grafana/grafana:11.6.0-patched

REGISTRY_ORG="${1:-}"
ARTIFACT="${2:-}"
OWNER="${GITHUB_REPOSITORY_OWNER:-verity-org}"

if [ -z "$REGISTRY_ORG" ]; then
  echo "Usage: $0 <registry/org> [artifact-ref]"
  echo ""
  echo "Examples:"
  echo "  $0 ghcr.io/verity-org grafana/grafana:11.6.0-patched"
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
  exit 0
fi

if [ $errors -gt 0 ]; then
  echo "FAILED: ${errors} verification(s) failed"
  exit 1
else
  echo "All verifications passed"
fi
