package chartgen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type chartgenFakeCommands struct {
	logPath     string
	packagePath string
}

func installChartgenCommandFakes(t *testing.T) chartgenFakeCommands {
	t.Helper()
	binDir := t.TempDir()
	stateDir := t.TempDir()
	fake := chartgenFakeCommands{
		logPath:     filepath.Join(stateDir, "commands.log"),
		packagePath: filepath.Join(stateDir, "sentinel-chart.tgz"),
	}

	writeChartgenExecutable(t, filepath.Join(binDir, "helm"), fakeHelmCommand)
	writeChartgenExecutable(t, filepath.Join(binDir, "crane"), fakeCraneCommand)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CHARTGEN_FAKE_LOG", fake.logPath)
	t.Setenv("CHARTGEN_FAKE_PACKAGE_PATH", fake.packagePath)
	t.Setenv("CHARTGEN_FAKE_HELM_MODE", "success")
	t.Setenv("CHARTGEN_FAKE_CRANE_MODE", "success")
	return fake
}

func writeChartgenExecutable(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
}

const fakeHelmCommand = `#!/bin/sh
set -eu
printf 'helm:%s\n' "$*" >> "$CHARTGEN_FAKE_LOG"
mode="${CHARTGEN_FAKE_HELM_MODE:-success}"

case "$1" in
template)
  if [ "$mode" = "template-fail" ]; then
    printf 'sentinel template failure\n' >&2
    exit 7
  fi
  case "$2" in
  mapped)
    printf '%s\n' \
      'apiVersion: v1' \
      'kind: Pod' \
      'spec:' \
      '  containers:' \
      '    - image: quay.io/acme/mapped:1.2.3' \
      '    - image: quay.io/acme/child:2.0.0'
    ;;
  excluded)
    printf '%s\n' \
      'apiVersion: v1' \
      'kind: Pod' \
      'spec:' \
      '  containers:' \
      '    - image: quay.io/acme/excluded:3.0.0'
    ;;
  missing)
    printf '%s\n' \
      'apiVersion: v1' \
      'kind: Pod' \
      'spec:' \
      '  containers:' \
      '    - image: quay.io/acme/missing:9.9.9'
    ;;
  *)
    printf '%s\n' 'apiVersion: v1' 'kind: ConfigMap' 'data: {}'
    ;;
  esac
  ;;
show)
  if [ "$mode" = "values-fail" ]; then
    printf 'sentinel values failure\n' >&2
    exit 8
  fi
  if [ "$mode" = "values-malformed" ]; then
    printf 'image: [\n'
    exit 0
  fi
  case "$3" in
  *mapped)
    printf '%s\n' \
      'image:' \
      '  repository: quay.io/acme/mapped' \
      '  tag: 1.2.3'
    ;;
  *)
    printf '%s\n' 'replicaCount: 1'
    ;;
  esac
  ;;
dependency)
  if [ "$mode" = "dependency-fail" ]; then
    printf 'sentinel dependency failure\n' >&2
    exit 9
  fi
  root="$3"
  if [ "$mode" = "dependency-charts-file" ]; then
    printf 'not a directory\n' > "$root/charts"
    exit 0
  fi
  mkdir -p "$root/charts/parent/charts/child"
  printf '%s\n' \
    'image:' \
    '  repository: quay.io/acme/child' \
    '  tag: 2.0.0' > "$root/charts/parent/charts/child/values.yaml"
  ;;
package)
  if [ "$mode" = "package-fail" ]; then
    printf 'sentinel package failure\n' >&2
    exit 10
  fi
  if [ "$mode" = "package-no-path" ]; then
    printf 'package completed without archive marker\n'
    exit 0
  fi
  printf 'sentinel archive\n' > "$CHARTGEN_FAKE_PACKAGE_PATH"
  printf 'noise before marker\n'
  printf 'Successfully packaged chart and saved it to: %s\n' "$CHARTGEN_FAKE_PACKAGE_PATH"
  ;;
push)
  if [ "$mode" = "push-fail" ]; then
    printf 'unauthorized sentinel push\n' >&2
    exit 11
  fi
  if [ "$mode" = "push-leave-directory" ]; then
    rm -f "$CHARTGEN_FAKE_PACKAGE_PATH"
    mkdir -p "$CHARTGEN_FAKE_PACKAGE_PATH"
    printf 'sentinel\n' > "$CHARTGEN_FAKE_PACKAGE_PATH/child"
  fi
  ;;
*)
  printf 'unexpected helm command: %s\n' "$*" >&2
  exit 12
  ;;
esac
`

const fakeCraneCommand = `#!/bin/sh
set -eu
printf 'crane:%s\n' "$*" >> "$CHARTGEN_FAKE_LOG"
if [ "${CHARTGEN_FAKE_CRANE_MODE:-success}" = "error" ]; then
  printf 'sentinel crane failure\n' >&2
  exit 13
fi

case "$2" in
*/acme/mapped)
  printf '%s\n' '0.9.0' '1.2.3'
  ;;
*/acme/child)
  printf '%s\n' '2.0.0'
  ;;
*/acme/missing)
  printf '%s\n' '9.9.8'
  ;;
*)
  printf '%s\n' 'latest'
  ;;
esac
`
