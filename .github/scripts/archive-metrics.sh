#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 4 ]; then
  echo "Usage: archive-metrics.sh <metrics-dir> <run-id> <run-attempt> <run-created-at>" >&2
  exit 2
fi

metrics_dir=$1
run_id=$2
run_attempt=$3
run_created_at=$4
attempts=${METRICS_ARCHIVE_ATTEMPTS:-5}
retry_delay=${METRICS_ARCHIVE_RETRY_DELAY_SECONDS:-2}

case "$attempts" in
  ''|*[!0-9]*|0*) echo "METRICS_ARCHIVE_ATTEMPTS must be a positive integer" >&2; exit 2 ;;
esac
case "$retry_delay" in
  ''|*[!0-9]*) echo "METRICS_ARCHIVE_RETRY_DELAY_SECONDS must be a non-negative integer" >&2; exit 2 ;;
esac

date=$(date -u -d "$run_created_at" +%Y-%m-%d)
target_dir="_metrics/runs/${date}/${run_id}-attempt-${run_attempt}"
start_ref=$(git rev-parse HEAD)
stash=$(mktemp -d)
trap 'rm -rf "$stash"' EXIT

shopt -s nullglob
metrics=("$metrics_dir"/metrics-*.json)
if [ "${#metrics[@]}" -eq 0 ]; then
  echo "No metrics JSON files found in ${metrics_dir}" >&2
  exit 1
fi
cp "${metrics[@]}" "$stash/"

for attempt in $(seq 1 "$attempts"); do
  echo "::group::Attempt ${attempt}"
  bootstrap_branch=""

  if git fetch origin '+refs/heads/_metrics:refs/remotes/origin/_metrics'; then
    git switch --detach refs/remotes/origin/_metrics
  elif git ls-remote --exit-code --heads origin _metrics >/dev/null 2>&1; then
    echo "::error::Failed to fetch existing _metrics branch"
    exit 1
  else
    bootstrap_branch="metrics-bootstrap-${run_id}-${run_attempt}-${attempt}"
    git switch --orphan "$bootstrap_branch"
    git rm -rf --ignore-unmatch .
  fi

  mkdir -p "$target_dir"
  for file in "$stash"/metrics-*.json; do
    base=$(basename "$file" .json)
    image=${base#metrics-}
    cp "$file" "$target_dir/${image}.json"
  done

  git add _metrics/
  if git diff --cached --quiet; then
    echo "::notice::No changes to commit"
    echo "::endgroup::"
    exit 0
  fi

  count=$(find "$target_dir" -maxdepth 1 -type f -name '*.json' | wc -l)
  git commit -m "metrics: run ${run_id} attempt ${run_attempt}" -m "${count} image(s)"

  if git push origin HEAD:refs/heads/_metrics; then
    echo "::notice::Pushed on attempt ${attempt}"
    echo "::endgroup::"
    exit 0
  fi

  git switch --detach "$start_ref"
  if [ -n "$bootstrap_branch" ]; then
    git branch -D "$bootstrap_branch"
  fi

  if [ "$attempt" -eq "$attempts" ]; then
    echo "::error::Failed to push metrics after ${attempts} attempts"
    echo "::endgroup::"
    exit 1
  fi

  echo "::warning::Push failed; refetching origin/_metrics before retry"
  echo "::endgroup::"
  sleep "$retry_delay"
done
