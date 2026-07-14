#!/usr/bin/env bash
set -euo pipefail

# Regression checks for wait-for-workflows.sh behavior that shellcheck/actionlint
# cannot prove.

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

cat > "$tmpdir/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

args=" $* "
case "$args" in
  *" api "*) ;;
  *) echo "fake gh only supports api" >&2; exit 2 ;;
esac

for required in "--method GET" "--paginate" "-f branch=main" "--jq"; do
  if [[ "$args" != *"$required"* ]]; then
    echo "missing required gh api argument: $required" >&2
    exit 1
  fi
done

status_arg_found=0
for arg in "$@"; do
  case "$arg" in
    status=*)
      echo "${arg#status=}" >> "${FAKE_GH_STATUSES:?}"
      status_arg_found=1
      ;;
  esac
done
if [ "$status_arg_found" -ne 1 ]; then
  echo "latest" >> "${FAKE_GH_CALLS:?}"
  printf '424242\t3\t%s\t2026-07-14T03:00:00Z\thttps://example.invalid/run\n' "${FAKE_GH_CONCLUSION:-success}"
  exit 0
fi

if [[ "$args" == *".database_id"* ]]; then
  echo "wait helper must use REST .id, not GraphQL .database_id" >&2
  exit 1
fi
if [[ "$args" != *".id"* ]]; then
  echo "wait helper must print the REST run id" >&2
  exit 1
fi

echo "active" >> "${FAKE_GH_CALLS:?}"
exit 0
EOF
chmod +x "$tmpdir/gh"

FAKE_GH_CALLS="$tmpdir/calls" \
FAKE_GH_STATUSES="$tmpdir/statuses" \
PATH="$tmpdir:$PATH" \
GITHUB_REPOSITORY="verity-org/verity" \
GITHUB_OUTPUT="$tmpdir/github-output" \
WAIT_BRANCH="main" \
WAIT_LOOKBACK_HOURS=1 \
WAIT_TIMEOUT_SECONDS=1 \
WAIT_INTERVAL_SECONDS=1 \
  bash .github/scripts/wait-for-workflows.sh integer-orchestrator.yaml > "$tmpdir/out"

if [ "$(grep -c '^active$' "$tmpdir/calls")" -ne 4 ] || [ "$(grep -c '^latest$' "$tmpdir/calls")" -ne 1 ]; then
  echo "wait helper should query active statuses and the latest completed producer" >&2
  exit 1
fi
if [ "$(sort -u "$tmpdir/statuses" | paste -sd, -)" != "in_progress,queued,requested,waiting" ]; then
  echo "wait helper must query queued, in_progress, requested, and waiting statuses" >&2
  exit 1
fi
if ! grep -q "No active producer workflow runs remain." "$tmpdir/out"; then
  echo "wait helper should exit cleanly when no active runs are returned" >&2
  exit 1
fi
if ! grep -qx "integer_orchestrator_batch_id=424242-3" "$tmpdir/github-output"; then
  echo "wait helper should publish the successful producer batch id" >&2
  exit 1
fi

if FAKE_GH_CONCLUSION="failure" \
  FAKE_GH_CALLS="$tmpdir/failed-calls" \
  FAKE_GH_STATUSES="$tmpdir/failed-statuses" \
  PATH="$tmpdir:$PATH" \
  GITHUB_REPOSITORY="verity-org/verity" \
  WAIT_BRANCH="main" \
  WAIT_LOOKBACK_HOURS=1 \
  WAIT_TIMEOUT_SECONDS=1 \
  WAIT_INTERVAL_SECONDS=1 \
    bash .github/scripts/wait-for-workflows.sh integer-orchestrator.yaml > "$tmpdir/failed-out" 2>&1; then
  echo "wait helper should fail when the latest producer failed" >&2
  exit 1
fi

echo "✓ wait-for-workflows validation passed"
