#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: download-previous-pages.sh OUTPUT_DIR" >&2
  exit 2
fi

output_dir="$1"
repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

if [[ -z "$output_dir" ]] || [[ "$output_dir" == "/" ]] || [[ "$output_dir" == "." ]] || [[ "/$output_dir/" == *"/../"* ]]; then
  echo "unsafe output directory: $output_dir" >&2
  exit 2
fi
if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI is required" >&2
  exit 2
fi
if ! command -v unzip >/dev/null 2>&1; then
  echo "unzip is required" >&2
  exit 2
fi

mapfile -t artifact_candidates < <(
  gh api "repos/${repo}/actions/artifacts" \
    --method GET \
    -f name=github-pages \
    -f per_page=100 \
    --jq '.artifacts
      | map(select(.expired == false and .workflow_run.head_branch == "main"))
      | sort_by(.created_at)
      | reverse
      | .[]
      | [.id, .workflow_run.id]
      | @tsv'
)

artifact_id=""
for candidate in "${artifact_candidates[@]}"; do
  IFS=$'\t' read -r candidate_artifact_id candidate_run_id <<< "$candidate"
  conclusion=$(gh api "repos/${repo}/actions/runs/${candidate_run_id}" --jq '.conclusion')
  if [[ "$conclusion" == "success" ]]; then
    artifact_id="$candidate_artifact_id"
    break
  fi
done

rm -rf "${output_dir:?}"
mkdir -p "$output_dir"
if [[ -z "$artifact_id" ]]; then
  echo "No retained successful main-branch Pages artifact found; APK repository will bootstrap from the candidate set"
  exit 0
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
gh api "repos/${repo}/actions/artifacts/${artifact_id}/zip" > "$tmpdir/pages.zip"
unzip -q "$tmpdir/pages.zip" -d "$tmpdir/download"
[[ -f "$tmpdir/download/artifact.tar" ]] || { echo "Pages artifact is missing artifact.tar" >&2; exit 1; }
tar -tf "$tmpdir/download/artifact.tar" > "$tmpdir/pages-files.txt"
if grep -qE '^\./apk(/|$)' "$tmpdir/pages-files.txt"; then
  tar -xf "$tmpdir/download/artifact.tar" -C "$output_dir" ./apk
fi

echo "Restored previous Pages artifact ${artifact_id}"
