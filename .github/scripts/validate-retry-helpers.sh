#!/usr/bin/env bash
set -euo pipefail

# Regression checks for retry helper behavior that shellcheck/actionlint cannot prove.

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

out=$(REGISTRY_COMMAND_ATTEMPTS=1 \
  bash .github/scripts/retry-registry-command.sh \
  "stdout clean" bash -c 'printf sha256:abc' 2>"$tmpdir/success.err")
if [ "$out" != "sha256:abc" ]; then
  echo "retry helper must preserve command stdout exactly" >&2
  exit 1
fi

if REGISTRY_COMMAND_ATTEMPTS=1 REGISTRY_COMMAND_BASE_DELAY_SECONDS=1 \
  bash .github/scripts/retry-registry-command.sh \
  "expected failure" bash -c 'exit 7' >"$tmpdir/failure.out" 2>"$tmpdir/failure.err"; then
  echo "retry helper must fail when the final command attempt fails" >&2
  exit 1
fi

if ! grep -q "exit 7" "$tmpdir/failure.err"; then
  echo "retry helper must report the command exit code from the failed attempt" >&2
  exit 1
fi

if REGISTRY_COMMAND_ATTEMPTS=00 \
  bash .github/scripts/retry-registry-command.sh \
  "bad attempts" true >"$tmpdir/bad-attempts.out" 2>"$tmpdir/bad-attempts.err"; then
  echo "retry helper must reject leading-zero attempt counts" >&2
  exit 1
fi

if REGISTRY_COMMAND_BASE_DELAY_SECONDS=00 \
  bash .github/scripts/retry-registry-command.sh \
  "bad delay" true >"$tmpdir/bad-delay.out" 2>"$tmpdir/bad-delay.err"; then
  echo "retry helper must reject leading-zero delay values" >&2
  exit 1
fi

echo "✓ retry helper validation passed"
