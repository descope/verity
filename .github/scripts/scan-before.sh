#!/usr/bin/env bash
set -euo pipefail

# Scans an image for vulnerabilities before patching.
# Inputs: SOURCE (env var) — full image reference
# Outputs: before_total, before_go, before_non_go (via GITHUB_ENV), before.json

: "${SOURCE:?SOURCE is required}"

trivy image \
  --severity CRITICAL,HIGH,MEDIUM,LOW \
  --scanners vuln \
  --format json \
  --output before.json \
  "$SOURCE"

TOTAL=$(jq '[.Results[]? | select(.Vulnerabilities) | .Vulnerabilities | length] | add // 0' before.json)
GO=$(jq '[.Results[]? | select(.Type == "gobinary") | select(.Vulnerabilities) | .Vulnerabilities | length] | add // 0' before.json)
# NON_GO: everything except gobinary (OS packages, Python libs, etc.)
NON_GO=$(jq '[.Results[]? | select(.Type != "gobinary") | select(.Vulnerabilities) | .Vulnerabilities | length] | add // 0' before.json)

{
  echo "before_total=${TOTAL}"
  echo "before_go=${GO}"
  echo "before_non_go=${NON_GO}"
} >> "$GITHUB_ENV"

echo "BEFORE — total: ${TOTAL}, non-go: ${NON_GO}, go: ${GO}"

if [ "$TOTAL" -eq 0 ]; then
  echo "::warning::No vulns found — image may have been patched upstream"
  echo "skip_patch=true" >> "$GITHUB_ENV"
fi
