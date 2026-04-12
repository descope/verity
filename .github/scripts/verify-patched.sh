#!/usr/bin/env bash
# shellcheck disable=SC2154 # before_total/before_go/before_os come from GITHUB_ENV
set -euo pipefail

# Scans a patched image and verifies 0 fixable OS CVEs remain.
# Inputs: PATCHED_IMAGE, IMAGE_LABEL (env vars), before_total/before_go/before_os (from GITHUB_ENV)
# Outputs: after.json, exit 1 if fixable OS CVEs remain (Go CVEs warn only)

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

if [ "$OS" -ne 0 ]; then
  echo "::error::Copa left ${OS} fixable OS CVEs unpatched for ${IMAGE_LABEL}"
  exit 1
fi

if [ "$GO" -ne 0 ]; then
  echo "::warning::${GO} fixable Go CVEs remain for ${IMAGE_LABEL} (Go binary patching is experimental)"
fi
