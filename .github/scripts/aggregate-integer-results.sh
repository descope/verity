#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 5 ]; then
  echo "Usage: aggregate-integer-results.sh <expected-json> <results-dir> <child-result> <repository> <run-id>" >&2
  exit 2
fi

expected_json=$1
results_dir=$2
child_result=$3
repository=$4
run_id=$5
run_url="https://github.com/${repository}/actions/runs/${run_id}"

if [ "$child_result" = "success" ] || [ "$child_result" = "skipped" ]; then
  echo "All dispatched Integer child builds succeeded."
  exit 0
fi

if ! jq -e 'type == "array" and all(.[]; (.name | type == "string") and (.version | type == "string") and (.type | type == "string"))' "$expected_json" >/dev/null; then
  echo "Invalid or missing Integer build plan; inspect ${run_url}" >&2
  exit 1
fi

reports=$(mktemp)
trap 'rm -f "$reports"' EXIT
mapfile -d '' report_files < <(find "$results_dir" -type f -name 'report.json' -print0 2>/dev/null || true)
if [ "${#report_files[@]}" -eq 0 ]; then
  printf '[]\n' > "$reports"
else
  jq -s '.' "${report_files[@]}" > "$reports"
fi

expected_count=$(jq 'length' "$expected_json")
missing_stage="matrix-dispatch-or-report"
if [ "$expected_count" -gt 256 ] && [ "${#report_files[@]}" -eq 0 ]; then
  missing_stage="matrix-expansion-limit"
  echo "Integer plan contains ${expected_count} entries; GitHub Actions permits at most 256 matrix jobs." >&2
fi

mapfile -t failures < <(
  jq -r \
    --arg missing_stage "$missing_stage" \
    --arg default_run_id "$run_id" \
    --slurpfile reports "$reports" '
    .[] as $expected |
    ($reports[0] | map(select(
      .image == $expected.name and
      .version == $expected.version and
      .type == $expected.type
    )) | first) as $report |
    if $report == null then
      [$expected.name, $expected.version, $expected.type,
       $missing_stage, $default_run_id, ($expected.name | gsub("/"; "-"))] | @tsv
    elif $report.status != "success" then
      [$expected.name, $expected.version, $expected.type,
       ($report.failure_stage // "unknown"),
       (if (($report.run_id // "") | length) > 0 then $report.run_id else $default_run_id end),
       ($expected.name | gsub("/"; "-"))] | @tsv
    else empty end
  ' "$expected_json"
)

echo "Integer child build matrix concluded: ${child_result}" >&2
if [ "${#failures[@]}" -eq 0 ]; then
  echo "No failed child report was available; inspect ${run_url}" >&2
  exit 1
fi

summary="## Failed Integer matrix entries"
for failure in "${failures[@]}"; do
  IFS=$'\t' read -r image version type stage report_run_id safe_image <<< "$failure"
  entry_run_id=${report_run_id:-$run_id}
  entry_url="https://github.com/${repository}/actions/runs/${entry_run_id}"
  artifact="integer-build-result-${safe_image}-${version}-${type}"
  line="- ${image}:${version}-${type} — stage=${stage}; run=${entry_url}; artifact=${artifact}"
  echo "$line" >&2
  summary+=$'\n'"$line"
done

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  printf '%s\n' "$summary" >> "$GITHUB_STEP_SUMMARY"
fi
exit 1
