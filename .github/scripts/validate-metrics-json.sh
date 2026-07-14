#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <run-id> <run-attempt> <metrics-directory>" >&2
  exit 2
fi

RUN_ID=$1
RUN_ATTEMPT=$2
METRICS_DIR=$3

if [[ ! "$RUN_ID" =~ ^[1-9][0-9]*$ ]] || [[ ! "$RUN_ATTEMPT" =~ ^[1-9][0-9]*$ ]]; then
  echo "run id and attempt must be positive integers" >&2
  exit 2
fi

if [ ! -d "$METRICS_DIR" ]; then
  echo "metrics directory does not exist: $METRICS_DIR" >&2
  exit 1
fi

shopt -s nullglob
files=("$METRICS_DIR"/metrics-*.json)
if [ "${#files[@]}" -eq 0 ]; then
  echo "No metrics artifacts found in $METRICS_DIR" >&2
  exit 1
fi

for file in "${files[@]}"; do
  if ! jq -e \
    --argjson expected_run_id "$RUN_ID" \
    --argjson expected_run_attempt "$RUN_ATTEMPT" '
      def integer: type == "number" and floor == .;
      def count: integer and . >= 0;
      def nonempty_string: type == "string" and length > 0;
      def nullable_string: . == null or nonempty_string;
      def digest: nonempty_string and test("^sha256:[0-9a-f]{64}$");
      def nullable_digest: . == null or digest;
      def severity_counts:
        type == "object"
        and (.CRITICAL | count)
        and (.HIGH | count)
        and (.MEDIUM | count)
        and (.LOW | count)
        and (.UNKNOWN | count);
      def scan:
        type == "object"
        and (.vuln_count | count)
        and (.by_severity | severity_counts)
        and .vuln_count == (
          .by_severity.CRITICAL
          + .by_severity.HIGH
          + .by_severity.MEDIUM
          + .by_severity.LOW
          + .by_severity.UNKNOWN
        );
      def platform($arch):
        . == null or (
          type == "object"
          and .arch == $arch
          and (.copa_duration_seconds == null or (.copa_duration_seconds | count))
          and (.copa_exit_code == null or (.copa_exit_code | integer))
          and (.staging_digest | nullable_digest)
        );

      .schema_version == "v1"
      and (.run.id == $expected_run_id)
      and (.run.attempt == $expected_run_attempt)
      and (.run.started_at | type == "string")
      and (.run.ended_at | nonempty_string)
      and (.run.conclusion | IN("success", "failure", "cancelled", "skipped"))
      and (.image.name | nonempty_string)
      and (.image.source_tag | nonempty_string)
      and (
        if .run.conclusion == "success" then
          (.image.target_ref | nonempty_string)
          and (.image.manifest_digest | digest)
          and (.scan.before | scan)
          and (.scan.after | scan)
        else
          (.image.target_ref | nullable_string)
          and (.image.manifest_digest | nullable_digest)
          and (.scan.before == null or (.scan.before | scan))
          and (.scan.after == null or (.scan.after | scan))
        end
      )
      and (.platforms.amd64 | platform("amd64"))
      and (.platforms.arm64 | platform("arm64"))
      and (.supply_chain.rekor_url | nullable_string)
      and (.supply_chain.attestation_id | nullable_string)
      and (.supply_chain.sbom_digest | nullable_digest)
      and (.supply_chain.attestation_bundle_path | nullable_string)
    ' "$file" >/dev/null; then
    echo "Invalid metrics JSON: $file" >&2
    exit 1
  fi
done

echo "Validated ${#files[@]} metrics file(s) for run $RUN_ID attempt $RUN_ATTEMPT"
