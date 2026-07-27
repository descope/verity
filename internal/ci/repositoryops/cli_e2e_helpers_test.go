//go:build e2e

package repositoryops_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type cliInvocation struct {
	binary      string
	arguments   []string
	environment []string
}

func buildVerityBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "verity")
	command := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	command.Dir = repositoryRoot(t)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return binary
}

func runCLI(t *testing.T, invocation cliInvocation) (string, error) {
	t.Helper()
	command := exec.CommandContext(t.Context(), invocation.binary, invocation.arguments...)
	command.Env = append(os.Environ(), invocation.environment...)
	output, err := command.CombinedOutput()
	return string(output), err
}

const fakeTrivyScript = `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$TRIVY_TRANSCRIPT"
output=''
previous=''
for argument in "$@"; do
  if [ "$previous" = '--output' ]; then
    output=$argument
    break
  fi
  previous=$argument
done
[ -n "$output" ]
printf '%s' "$TRIVY_REPORT" > "$output"
`

const fakeGitScript = `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$GIT_TRANSCRIPT"
if [ "${FAKE_GIT_MUTATE_PATH:-}" != '' ]; then
  case "$*" in
    checkout\ -b\ *)
      printf '%s\n' overwritten > "$FAKE_GIT_MUTATE_PATH"
      printf '%s\n' created > "$FAKE_GIT_CREATED_PATH"
      ;;
  esac
fi
if [ "${FAKE_GIT_FAIL_MODE:-}" = 'commit' ]; then
  case "$*" in
    commit\ -m\ *) printf '%s\n' 'forced commit failure' >&2; exit 96 ;;
  esac
fi
case "${FAKE_GIT_MODE}:$*" in
  "add:status --porcelain=v1 -z --untracked-files=all") exit 0 ;;
  add:show-ref\ --verify\ --quiet\ refs/heads/add-image/*) exit 1 ;;
  "add:rev-parse --verify HEAD") printf '%s\n' '1111111111111111111111111111111111111111' ;;
  "add:symbolic-ref -q HEAD") printf '%s\n' 'refs/heads/main' ;;
  add:rev-parse\ --verify\ --quiet\ refs/heads/add-image/*)
    if [ -f "$PWD/.git/verity-automation-ref" ]; then cat "$PWD/.git/verity-automation-ref"; else exit 1; fi
    ;;
  add:rev-parse\ --verify\ --quiet\ refs/remotes/origin/add-image/*) exit 1 ;;
  add:ls-remote\ --refs\ origin\ refs/heads/add-image/*) exit 0 ;;
  "add:rev-parse --path-format=absolute --git-dir") printf '%s/.git\n' "$PWD" ;;
  "add:rev-parse --path-format=absolute --git-common-dir") printf '%s/.git\n' "$PWD" ;;
  "add:rev-parse --path-format=absolute --git-path HEAD") printf '%s/.git/HEAD\n' "$PWD" ;;
  "add:rev-parse --path-format=absolute --git-path index") printf '%s/.git/index\n' "$PWD" ;;
  "add:rev-parse --path-format=absolute --git-path config") printf '%s/.git/config\n' "$PWD" ;;
  "add:update-ref --stdin") cat >/dev/null; rm -f "$PWD/.git/verity-automation-ref" ;;
  add:checkout\ -b\ *) printf '%s\n' '2222222222222222222222222222222222222222' > "$PWD/.git/verity-automation-ref" ;;
  add:config\ user.*|add:add\ --\ *|add:commit\ -m\ *|add:push\ -u\ origin\ *) exit 0 ;;
  "sync:diff --cached --quiet --") exit 0 ;;
  "sync:status --porcelain=v1 -z --untracked-files=all") printf '?? images/a.yaml\000 M images/z.yaml\000' ;;
  "sync:rev-parse HEAD") printf '%s\n' '1111111111111111111111111111111111111111' ;;
  "sync:ls-remote --exit-code origin refs/heads/main") printf '%s\t%s\n' '1111111111111111111111111111111111111111' 'refs/heads/main' ;;
  sync:restore\ --\ *|sync:add\ --\ *|sync:config\ user.*|sync:commit\ -m\ *|sync:push\ --force-with-lease=*) exit 0 ;;
  "sync:diff --cached --quiet --exit-code") exit 1 ;;
  "sync:ls-remote --exit-code origin refs/heads/automation/integer-package-streams") exit 2 ;;
  *) printf 'unexpected git command: %s\n' "$*" >&2; exit 97 ;;
esac
`

const fakeGitHubScript = `#!/bin/sh
set -eu
printf 'token=%s command=%s\n' "$GH_TOKEN" "$*" >> "$GH_TRANSCRIPT"
case "$*" in
  pr\ list\ *) exit 0 ;;
  pr\ create\ *) printf '%s\n' "${FAKE_GH_URL:-https://github.com/verity-org/verity/pull/42}" ;;
  *) printf 'unexpected gh command: %s\n' "$*" >&2; exit 98 ;;
esac
`
