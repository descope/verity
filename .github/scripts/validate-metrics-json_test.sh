#!/usr/bin/env bash
set -euo pipefail

# Regression checks for archived metrics validation.

ROOT=$(git rev-parse --show-toplevel)
VALIDATOR="$ROOT/.github/scripts/validate-metrics-json.sh"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

expect_failure() {
  local label=$1
  shift
  if "$@" >/dev/null 2>&1; then
    echo "expected failure: $label" >&2
    exit 1
  fi
}

mkdir -p "$TMPDIR/valid"
cat > "$TMPDIR/valid/metrics-example-1.2.3.json" <<'JSON'
{
  "schema_version": "v1",
  "run": {
    "id": 42,
    "attempt": 3,
    "started_at": "2026-07-14T00:00:00Z",
    "ended_at": "2026-07-14T00:01:00Z",
    "conclusion": "success"
  },
  "image": {
    "name": "example",
    "source_tag": "1.2.3",
    "target_ref": "ghcr.io/verity-org/example:1.2.3",
    "manifest_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },
  "scan": {
    "before": {
      "vuln_count": 3,
      "by_severity": {"CRITICAL": 1, "HIGH": 1, "MEDIUM": 1, "LOW": 0, "UNKNOWN": 0}
    },
    "after": {
      "vuln_count": 1,
      "by_severity": {"CRITICAL": 0, "HIGH": 1, "MEDIUM": 0, "LOW": 0, "UNKNOWN": 0}
    }
  },
  "platforms": {
    "amd64": {"arch": "amd64", "copa_duration_seconds": 12, "copa_exit_code": 0, "staging_digest": null},
    "arm64": null
  },
  "supply_chain": {
    "rekor_url": null,
    "attestation_id": null,
    "sbom_digest": null,
    "attestation_bundle_path": null
  }
}
JSON

bash "$VALIDATOR" 42 3 "$TMPDIR/valid"

mkdir -p "$TMPDIR/failure"
jq '
  .run.conclusion = "failure"
  | .image.target_ref = null
  | .image.manifest_digest = null
  | .scan.before = null
  | .scan.after = null
' "$TMPDIR/valid/metrics-example-1.2.3.json" \
  > "$TMPDIR/failure/metrics-example-1.2.3.json"
bash "$VALIDATOR" 42 3 "$TMPDIR/failure"

mkdir -p "$TMPDIR/null-target"
jq '.image.target_ref = null' "$TMPDIR/valid/metrics-example-1.2.3.json" \
  > "$TMPDIR/null-target/metrics-example-1.2.3.json"
expect_failure "successful record without target" \
  bash "$VALIDATOR" 42 3 "$TMPDIR/null-target"

mkdir -p "$TMPDIR/wrong-run"
cp "$TMPDIR/valid/metrics-example-1.2.3.json" \
  "$TMPDIR/wrong-run/metrics-example-1.2.3.json"
expect_failure "record from another workflow run" \
  bash "$VALIDATOR" 43 3 "$TMPDIR/wrong-run"

mkdir -p "$TMPDIR/bad-total"
jq '.scan.after.vuln_count = 2' "$TMPDIR/valid/metrics-example-1.2.3.json" \
  > "$TMPDIR/bad-total/metrics-example-1.2.3.json"
expect_failure "severity total mismatch" \
  bash "$VALIDATOR" 42 3 "$TMPDIR/bad-total"

mkdir -p "$TMPDIR/multi-document"
{
  echo '{}'
  cat "$TMPDIR/valid/metrics-example-1.2.3.json"
} > "$TMPDIR/multi-document/metrics-example-1.2.3.json"
expect_failure "multiple JSON documents" \
  bash "$VALIDATOR" 42 3 "$TMPDIR/multi-document"

mkdir -p "$TMPDIR/missing-platforms"
jq 'del(.platforms)' "$TMPDIR/failure/metrics-example-1.2.3.json" \
  > "$TMPDIR/missing-platforms/metrics-example-1.2.3.json"
expect_failure "missing platforms container" \
  bash "$VALIDATOR" 42 3 "$TMPDIR/missing-platforms"

mkdir -p "$TMPDIR/missing-supply-chain"
jq 'del(.supply_chain)' "$TMPDIR/failure/metrics-example-1.2.3.json" \
  > "$TMPDIR/missing-supply-chain/metrics-example-1.2.3.json"
expect_failure "missing supply-chain container" \
  bash "$VALIDATOR" 42 3 "$TMPDIR/missing-supply-chain"

mkdir -p "$TMPDIR/empty-started-at"
jq '.run.started_at = ""' "$TMPDIR/valid/metrics-example-1.2.3.json" \
  > "$TMPDIR/empty-started-at/metrics-example-1.2.3.json"
expect_failure "empty start timestamp" \
  bash "$VALIDATOR" 42 3 "$TMPDIR/empty-started-at"

mkdir -p "$TMPDIR/empty"
expect_failure "missing metrics files" bash "$VALIDATOR" 42 3 "$TMPDIR/empty"

echo "metrics JSON validation checks passed"
