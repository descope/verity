#!/bin/bash
# Sign and attest all patched images from Copa bulk config
set -euo pipefail

COPA_CONFIG="${1:-copa-config.yaml}"
REGISTRY="${2:-ghcr.io/verity-org}"
REPORTS_DIR="${3:-.verity/reports}"
IMAGES_JSON="${4:-.verity/images.json}"
SKIP_SIGNING="${5:-false}"

# Check if --skip-signing flag is present in any argument
for arg in "$@"; do
  if [ "$arg" = "--skip-signing" ]; then
    SKIP_SIGNING="true"
    break
  fi
done

mkdir -p "$REPORTS_DIR"
images_list="[]"

if [ "$SKIP_SIGNING" = "true" ]; then
  echo "Running in scan-only mode (no signing/attestation)"
fi

# Extract target repos from copa-config.yaml
# Copa keeps only the last path segment: quay.io/foo/bar → $REGISTRY/bar
for source_image in $(yq -r '.images[].image' "$COPA_CONFIG"); do
  # Derive the base repository name (last path segment, without tag or digest)
  base_with_tag_and_digest="${source_image##*/}"   # remove everything up to last '/'
  base_without_digest="${base_with_tag_and_digest%%@*}"  # strip optional @digest
  base_name="${base_without_digest%%:*}"           # strip optional :tag
  target_repo="${REGISTRY}/${base_name}"

  # List all patched tags in the target registry
  tags=$(crane ls "$target_repo" 2>/dev/null | grep -- '-patched' || true)
  [ -z "$tags" ] && continue

  for tag in $tags; do
    ref="${target_repo}:${tag}"
    digest=$(crane digest "$ref" 2>/dev/null) || continue
    image_name="${target_repo}"

    echo "Processing ${ref}@${digest}"

    # 1. Trivy scan → vulnerability report
    report_file="${REPORTS_DIR}/${base_name}_${tag}.json"
    if ! trivy image --format json --output "$report_file" "${image_name}@${digest}"; then
      echo "Warning: Trivy scan failed for ${image_name}@${digest}, skipping vulnerability attestation for this image." >&2
    fi

    if [ "$SKIP_SIGNING" != "true" ]; then
      # 2. Sign with cosign
      cosign sign --yes "${image_name}@${digest}"

      # 3. Generate + attest SBOM
      sbom_file="${REPORTS_DIR}/${base_name}_${tag}.sbom.cdx.json"
      trivy image --format cyclonedx --output "$sbom_file" "${image_name}@${digest}"
      cosign attest --yes --predicate "$sbom_file" --type cyclonedx "${image_name}@${digest}"

      # 4. Attest vulnerability report
      if [ -f "$report_file" ]; then
        cosign attest --yes --predicate "$report_file" --type vuln "${image_name}@${digest}"
      fi
    else
      echo "  ✓ Scanned (signing skipped for PR testing)"
    fi

    # 5. Derive original ref from patched tag (strip -patched suffix)
    # Handle tags like:
    #   v3.9.1-patched        → v3.9.1
    #   v3.9.1-patched-2      → v3.9.1
    #   v3.9.1-patched-12     → v3.9.1
    if [[ "$tag" =~ ^(.+)-patched-[0-9]+$ ]]; then
      original_tag="${BASH_REMATCH[1]}"
    else
      original_tag="${tag%-patched}"
    fi
    original_ref="${source_image}:${original_tag}"

    # Accumulate for catalog
    images_list=$(echo "$images_list" | jq \
      --arg orig "$original_ref" \
      --arg patched "$ref" \
      --arg report "$report_file" \
      '. + [{"original": $orig, "patched": $patched, "report": $report}]')
  done
done

# Write images list for catalog generation
echo "$images_list" | jq '.' > "$IMAGES_JSON"
echo "Processed $(echo "$images_list" | jq length) images → $IMAGES_JSON"
