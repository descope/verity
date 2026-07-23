#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Usage: download-approved-apks.sh RUN_ID-RUN_ATTEMPT OUTPUT_DIR" >&2
  exit 2
fi

batch_id="$1"
output_dir="$2"
repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

if [[ ! "$batch_id" =~ ^[0-9]+-[1-9][0-9]*$ ]]; then
  echo "batch ID must have the form RUN_ID-RUN_ATTEMPT: $batch_id" >&2
  exit 2
fi
if [[ -z "$output_dir" ]] || [[ "$output_dir" == "/" ]] || [[ "$output_dir" == "." ]] || [[ "/$output_dir/" == *"/../"* ]]; then
  echo "unsafe output directory: $output_dir" >&2
  exit 2
fi
if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI is required" >&2
  exit 2
fi

run_id="${batch_id%%-*}"
rm -rf "${output_dir:?}"
mkdir -p "$output_dir"

gh run download "$run_id" \
  --repo "$repo" \
  --pattern "apk-repository-${batch_id}-*" \
  --dir "$output_dir"

mapfile -t apks < <(find "$output_dir" -type f -name '*.apk' -print | sort)
if [[ ${#apks[@]} -eq 0 ]]; then
  echo "trusted Integer run $batch_id did not publish approved APK artifacts" >&2
  exit 1
fi

signer_workflow="github.com/${repo}/.github/workflows/integer-build-image.yaml"
for apk_file in "${apks[@]}"; do
  gh attestation verify "$apk_file" \
    --repo "$repo" \
    --signer-workflow "$signer_workflow" \
    --source-ref refs/heads/main \
    --deny-self-hosted-runners >/dev/null
done

echo "Downloaded and verified ${#apks[@]} approved APK packages from Integer batch $batch_id"
