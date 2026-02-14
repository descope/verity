#!/bin/bash
set -euo pipefail

# Ensure a Quay.io repository is public.
# Usage: quay-make-public.sh <namespace> <repo-name>
# Requires QUAY_PASSWORD (robot account token used as Bearer token).

NAMESPACE="${1:?Usage: quay-make-public.sh <namespace> <repo-name>}"
REPO="${2:?Usage: quay-make-public.sh <namespace> <repo-name>}"
TOKEN="${QUAY_PASSWORD:-}"

if [ -z "$TOKEN" ]; then
  echo "  Warning: QUAY_PASSWORD not set, skipping visibility update for ${NAMESPACE}/${REPO}"
  exit 0
fi

HTTP_CODE=$(curl -s -o /tmp/quay-visibility-response -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"visibility": "public"}' \
  "https://quay.io/api/v1/repository/${NAMESPACE}/${REPO}/changevisibility")

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
