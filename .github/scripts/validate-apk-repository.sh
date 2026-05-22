#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: validate-apk-repository.sh [--require-signature] REPO_DIR

Validates a static APK repository layout:
  REPO_DIR/<arch>/*.apk
  REPO_DIR/<arch>/APKINDEX.tar.gz
  REPO_DIR/*.rsa.pub when signatures are required

If REPO_DIR/.no-apks-found exists and no APKs are present, validation passes.
USAGE
}

REQUIRE_SIGNATURE=false
REPO_DIR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --require-signature)
      REQUIRE_SIGNATURE=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      echo "unknown option: $1" >&2
      usage
      exit 2
      ;;
    *)
      if [[ -n "$REPO_DIR" ]]; then
        echo "unexpected extra argument: $1" >&2
        exit 2
      fi
      REPO_DIR="$1"
      shift
      ;;
  esac
done

[[ -n "$REPO_DIR" ]] || { usage; exit 2; }
[[ -d "$REPO_DIR" ]] || { echo "repository directory not found: $REPO_DIR" >&2; exit 1; }

mapfile -t ROOT_APKS < <(find "$REPO_DIR" -maxdepth 1 -type f -name '*.apk' -print | sort)
if [[ ${#ROOT_APKS[@]} -gt 0 ]]; then
  echo "APK files must live under architecture directories, not repository root:" >&2
  printf '  %s\n' "${ROOT_APKS[@]}" >&2
  exit 1
fi

mapfile -t APKS < <(find "$REPO_DIR" -mindepth 2 -type f -name '*.apk' -print | sort)
if [[ ${#APKS[@]} -eq 0 ]]; then
  if [[ -f "$REPO_DIR/.no-apks-found" ]]; then
    echo "No APK files present; guarded empty repository marker found"
    exit 0
  fi
  echo "no APK files found and .no-apks-found marker is missing" >&2
  exit 1
fi

if [[ "$REQUIRE_SIGNATURE" == "true" ]]; then
  mapfile -t PUBKEYS < <(find "$REPO_DIR" -maxdepth 1 -type f -name '*.rsa.pub' -print | sort)
  if [[ ${#PUBKEYS[@]} -eq 0 ]]; then
    echo "signature required but no public key (*.rsa.pub) found at repository root" >&2
    exit 1
  fi
fi

status=0
for arch_dir in "$REPO_DIR"/*; do
  [[ -d "$arch_dir" ]] || continue
  shopt -s nullglob
  arch_apks=("$arch_dir"/*.apk)
  shopt -u nullglob
  [[ ${#arch_apks[@]} -gt 0 ]] || continue

  index="$arch_dir/APKINDEX.tar.gz"
  if [[ ! -f "$index" ]]; then
    echo "missing APKINDEX.tar.gz for ${arch_dir#"${REPO_DIR}"/}" >&2
    status=1
    continue
  fi

  if ! tar -tzf "$index" >/dev/null 2>&1; then
    echo "invalid gzip tar index: $index" >&2
    status=1
    continue
  fi

  if [[ "$REQUIRE_SIGNATURE" == "true" ]] && \
     ! tar -tzf "$index" | grep -Eq '(^|/)\.SIGN\.RSA\..*\.rsa\.pub$'; then
    echo "missing RSA signature entry in $index" >&2
    status=1
  fi
done

if [[ $status -ne 0 ]]; then
  exit "$status"
fi

echo "APK repository layout valid: $REPO_DIR (${#APKS[@]} packages)"
