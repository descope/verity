#!/usr/bin/env bash
set -euo pipefail

readonly advisory_id=GO-2026-5939
readonly vulnerable_package=github.com/quay/claircore/libindex

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
result="$tmpdir/govulncheck.txt"
metadata="$tmpdir/govulncheck.json"
dependencies="$tmpdir/dependencies.txt"

set +e
govulncheck ./... >"$result" 2>&1
status=$?
set -e
cat "$result"

if [ "$status" -eq 0 ]; then
  exit 0
fi
if [ "$status" -ne 3 ]; then
  echo "govulncheck failed before completing the vulnerability scan" >&2
  exit "$status"
fi

source_findings=$(sed -nE \
  's/^Vulnerability #[0-9]+: (GO-[0-9]{4}-[0-9]+)$/\1/p' \
  "$result" | sort -u)
if [ "$source_findings" != "$advisory_id" ]; then
  echo "govulncheck found a source vulnerability outside the scoped advisory" >&2
  exit "$status"
fi

if ! command -v jq >/dev/null; then
  echo "jq is required to inspect $advisory_id vulnerability metadata" >&2
  exit "$status"
fi
if ! govulncheck -format json ./... >"$metadata"; then
  echo "unable to inspect the $advisory_id vulnerability metadata" >&2
  exit "$status"
fi
if ! jq -s -e --arg id "$advisory_id" '
  [.[] | select(.osv.id == $id) | .osv] as $records |
  ($records | length) == 1 and
  $records[0].database_specific.review_status == "UNREVIEWED" and
  ($records[0].affected | length) == 1 and
  $records[0].affected[0].package.name == "github.com/quay/claircore" and
  $records[0].affected[0].ranges == [{
    "type": "SEMVER",
    "events": [{"introduced": "0"}]
  }] and
  (($records[0].affected[0].ecosystem_specific.imports // []) | length) == 0
' "$metadata" >/dev/null; then
  echo "$advisory_id metadata changed; the not-affected decision requires review" >&2
  exit "$status"
fi

if ! go list -deps ./... >"$dependencies"; then
  echo "unable to prove that the GO-2026-5939 fetcher package is absent" >&2
  exit "$status"
fi
if grep -Fxq "$vulnerable_package" "$dependencies"; then
  echo "$advisory_id affects the reachable package $vulnerable_package" >&2
  exit "$status"
fi

echo "$advisory_id: not affected; $vulnerable_package is absent from the dependency graph" >&2
