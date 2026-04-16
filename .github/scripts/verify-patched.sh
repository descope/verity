#!/usr/bin/env bash
# shellcheck disable=SC2154 # before_total/before_go/before_non_go come from GITHUB_ENV
set -euo pipefail

# Scans a patched image and verifies 0 fixable non-Go CVEs remain.
# Inputs: PATCHED_IMAGE, IMAGE_LABEL (env vars), before_total/before_go/before_non_go (from GITHUB_ENV)
# Outputs: after.json, exit 1 if fixable non-Go CVEs remain (Go CVEs warn only)

: "${PATCHED_IMAGE:?PATCHED_IMAGE is required}"
: "${IMAGE_LABEL:?IMAGE_LABEL is required}"

trivy image \
  --severity CRITICAL,HIGH,MEDIUM,LOW \
  --ignore-unfixed \
  --scanners vuln \
  --format json \
  --output after.json \
  "$PATCHED_IMAGE"

TOTAL=$(jq '[.Results[]? | select(.Vulnerabilities) | .Vulnerabilities | length] | add // 0' after.json)
GO=$(jq '[.Results[]? | select(.Type == "gobinary") | select(.Vulnerabilities) | .Vulnerabilities | length] | add // 0' after.json)
# NON_GO: everything except gobinary (OS packages, Python libs, etc.)
NON_GO=$(jq '[.Results[]? | select(.Type != "gobinary") | select(.Vulnerabilities) | .Vulnerabilities | length] | add // 0' after.json)

{
  echo ""
  echo "══════════════════════════════════════════════"
  echo "  ${IMAGE_LABEL}"
  echo "──────────────────────────────────────────────"
  echo "  BEFORE:  ${before_total} total (${before_non_go} non-Go, ${before_go} Go)"
  echo "  AFTER:   ${TOTAL} total (${NON_GO} non-Go, ${GO} Go) [fixable only]"
  echo "  FIXED:   $((before_total - TOTAL)) total"
  echo "══════════════════════════════════════════════"
}

if [ "$NON_GO" -ne 0 ]; then
  echo "::error::Copa left ${NON_GO} fixable non-Go CVEs unpatched for ${IMAGE_LABEL} (${NON_GO} non-Go, ${GO} Go)"
  exit 1
fi

if [ "$GO" -ne 0 ]; then
  echo "::warning::${GO} fixable Go CVEs remain for ${IMAGE_LABEL} (Go binary patching is best-effort)"
fi
