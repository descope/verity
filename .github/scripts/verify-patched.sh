#!/usr/bin/env bash
set -euo pipefail

# before_total, before_go, before_os are set by scan-before.sh via GITHUB_ENV
# shellcheck disable=SC2154
# Scans a patched image and verifies 0 fixable CVEs remain.
# Inputs: PATCHED_IMAGE, IMAGE_LABEL (env vars), before_total/before_go/before_os (from GITHUB_ENV)
# Outputs: after.json, exit 1 if fixable CVEs remain

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
OS=$(jq '[.Results[]? | select(.Type != "gobinary") | select(.Vulnerabilities) | .Vulnerabilities | length] | add // 0' after.json)

{
  echo ""
  echo "══════════════════════════════════════════════"
  echo "  ${IMAGE_LABEL}"
  echo "──────────────────────────────────────────────"
  echo "  BEFORE:  ${before_total} total (${before_os} OS, ${before_go} Go)"
  echo "  AFTER:   ${TOTAL} total (${OS} OS, ${GO} Go) [fixable only]"
  echo "  FIXED:   $((before_total - TOTAL)) total"
  echo "══════════════════════════════════════════════"
}

if [ "$TOTAL" -ne 0 ]; then
  echo "::error::Copa left ${TOTAL} fixable CVEs unpatched for ${IMAGE_LABEL} (${OS} OS, ${GO} Go)"
  exit 1
fi
