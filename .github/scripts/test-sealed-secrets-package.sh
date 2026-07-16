#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <x86_64|aarch64>" >&2
  exit 2
fi

arch=$1
case "$arch" in
  x86_64 | aarch64) ;;
  *)
    echo "unsupported Sealed Secrets package architecture: $arch" >&2
    exit 2
    ;;
esac

workspace=${GITHUB_WORKSPACE:-$(pwd)}
cd "$workspace"
timeout --signal=TERM --kill-after=1m 30m melange test \
  --arch "$arch" \
  --repository-append "$workspace/packages/repo" \
  --repository-append https://packages.wolfi.dev/os \
  --keyring-append "$workspace/melange-work/melange.rsa.pub" \
  --keyring-append https://packages.wolfi.dev/os/wolfi-signing.rsa.pub \
  --runner docker \
  --pipeline-dirs melange-work/pipelines \
  melange-work/specs/sealed-secrets-0.yaml/build.yaml \
  sealed-secrets-0
