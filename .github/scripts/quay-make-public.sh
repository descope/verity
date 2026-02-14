#!/bin/bash
set -euo pipefail

# Ensure a Quay.io repository is public.
# Usage: quay-make-public.sh <namespace> <repo-name>
# Requires QUAY_API_TOKEN (OAuth Application Token with "Administer Repositories" scope).
# Robot account passwords do NOT work for the Quay REST API.

NAMESPACE="${1:?Usage: quay-make-public.sh <namespace> <repo-name>}"
REPO="${2:?Usage: quay-make-public.sh <namespace> <repo-name>}"
TOKEN="${QUAY_API_TOKEN:-}"

if [ -z "$TOKEN" ]; then
  echo "  Warning: QUAY_API_TOKEN not set, skipping visibility update for ${NAMESPACE}/${REPO}"
  exit 0
fi

# URL-encode the repo name (replace / with %2F) for repos like "charts/prometheus".
ENCODED_REPO="${REPO//\//%2F}"

HTTP_CODE=$(curl -s -o /tmp/quay-visibility-response -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"visibility": "public"}' \
  "https://quay.io/api/v1/repository/${NAMESPACE}/${ENCODED_REPO}/changevisibility")

case "$HTTP_CODE" in
  200)
    echo "  Repository ${NAMESPACE}/${REPO} set to public"
    ;;
  *)
    echo "  Warning: could not set ${NAMESPACE}/${REPO} to public (HTTP ${HTTP_CODE})"
    cat /tmp/quay-visibility-response 2>/dev/null || true
    echo ""
    ;;
esac
