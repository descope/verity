#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 6 ]; then
  echo "Usage: aggregate-integer-results.sh <expected-json> <results-dir> <child-result> <repository> <run-id> <batch-id>" >&2
  exit 2
fi

expected_json=$1
results_dir=$2
child_result=$3
repository=$4
run_id=$5
batch_id=$6
run_url="https://github.com/${repository}/actions/runs/${run_id}"
shard_size=250

if ! jq -e '
  type == "array"
  and all(.[];
    (.name | type == "string")
    and (.version | type == "string")
    and (.type | type == "string")
  )
  and ((map([.name, .version, .type] | @tsv) | length)
    == (map([.name, .version, .type] | @tsv) | unique | length))
' "$expected_json" >/dev/null; then
  echo "Invalid or duplicate Integer build plan; inspect ${run_url}" >&2
  exit 1
fi

expected_count=$(jq 'length' "$expected_json")
if [ "$expected_count" -eq 0 ]; then
  if [ "$child_result" = "skipped" ]; then
    echo "No Integer child builds were dispatched."
    exit 0
  fi
  echo "Empty Integer plan concluded ${child_result}, expected skipped; inspect ${run_url}" >&2
  exit 1
fi

reports=$(mktemp)
trap 'rm -f "$reports"' EXIT
mapfile -d '' report_files < <(find "$results_dir" -type f -name 'report.json' -print0 2>/dev/null || true)
if [ "${#report_files[@]}" -eq 0 ]; then
  printf '[]\n' > "$reports"
elif ! jq -s '.' "${report_files[@]}" > "$reports"; then
  echo "Invalid Integer child report JSON; inspect ${run_url}" >&2
  exit 1
fi

mapfile -t failures < <(
  jq -r \
    --arg batch_id "$batch_id" \
    --arg default_run_id "$run_id" \
    --argjson shard_size "$shard_size" \
    --slurpfile reports "$reports" '
    def same_entry($report; $expected):
      $report.image == $expected.name
      and $report.version == $expected.version
      and $report.type == $expected.type;
    def failure_row($entry; $shard; $stage; $report):
      [$entry.name, $entry.version, $entry.type, ($shard | tostring), $stage,
       (if (($report.run_id // "") | length) > 0 then $report.run_id else $default_run_id end),
       ($entry.name | gsub("/"; "-"))];

    . as $expected_entries |
    $reports[0] as $all_reports |
    [
      ($expected_entries | to_entries[] |
        .key as $index |
        .value as $expected |
        (($index / $shard_size | floor) + 1) as $expected_shard |
        ($all_reports | map(select(same_entry(.; $expected)))) as $identity_reports |
        ($identity_reports | map(select((.batch_id // "") == $batch_id))) as $current_reports |
        if ($current_reports | length) == 0 then
          ($identity_reports | first) as $stale |
          failure_row(
            $expected;
            $expected_shard;
            (if $stale == null then "matrix-dispatch-or-report" else "batch-mismatch" end);
            ($stale // {})
          )
        elif ($current_reports | length) > 1 then
          failure_row($expected; $expected_shard; "duplicate-child-report"; $current_reports[0])
        elif (($current_reports[0].shard // 0) | tostring) != ($expected_shard | tostring) then
          failure_row($expected; $expected_shard; "wrong-shard-report"; $current_reports[0])
        elif $current_reports[0].status != "success" then
          failure_row(
            $expected;
            $expected_shard;
            ($current_reports[0].failure_stage // "unknown");
            $current_reports[0]
          )
        else empty end
      ),
      ($all_reports[] as $report |
        select(($report.batch_id // "") == $batch_id) |
        select(($expected_entries | map(select(same_entry($report; .))) | length) == 0) |
        failure_row(
          {name: ($report.image // "unknown"), version: ($report.version // "unknown"), type: ($report.type // "unknown")};
          ($report.shard // 0);
          "unexpected-child-report";
          $report
        )
      )
    ] | .[] | @tsv
  ' "$expected_json"
)

echo "Integer child shard matrix concluded: ${child_result}" >&2
if [ "${#failures[@]}" -eq 0 ]; then
  if [ "$child_result" != "success" ]; then
    echo "No failed child report explains shard result ${child_result}; batch=${batch_id}; inspect ${run_url}" >&2
    exit 1
  fi
  shard_count=$(((expected_count + shard_size - 1) / shard_size))
  echo "All ${expected_count} planned Integer child builds succeeded across ${shard_count} shard(s)."
  exit 0
fi

summary="## Failed Integer matrix entries"
for failure in "${failures[@]}"; do
  IFS=$'\t' read -r image version type shard stage report_run_id safe_image <<< "$failure"
  entry_run_id=${report_run_id:-$run_id}
  entry_url="https://github.com/${repository}/actions/runs/${entry_run_id}"
  artifact="integer-build-result-${safe_image}-${version}-${type}"
  line="- ${image}:${version}-${type} — shard=${shard}; stage=${stage}; batch=${batch_id}; run=${entry_url}; artifact=${artifact}"
  echo "$line" >&2
  summary+=$'\n'"$line"
done

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  printf '%s\n' "$summary" >> "$GITHUB_STEP_SUMMARY"
fi
exit 1
