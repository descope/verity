#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: assemble-apk-repository.sh [--output DIR] [--key-name NAME] [SOURCE_DIR ...]

Collects .apk files from SOURCE_DIRs, lays them out as a static APK repository,
builds APKINDEX.tar.gz per architecture, and signs each index when
APK_REPOSITORY_PRIVATE_KEY is set.

Defaults:
  output: site/dist/apk
  sources: packages/repo apk-artifacts
  key name: verity-apk-repository.rsa

Guarded behavior: if no .apk files are found, writes .no-apks-found and exits 0.
USAGE
}

OUTPUT_DIR="site/dist/apk"
KEY_NAME="verity-apk-repository.rsa"
SOURCES=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output)
      [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --key-name)
      [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }
      KEY_NAME="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "unknown option: $1" >&2
      usage
      exit 2
      ;;
    *)
      SOURCES+=("$1")
      shift
      ;;
  esac
done

if [[ $# -gt 0 ]]; then
  SOURCES+=("$@")
fi
if [[ ${#SOURCES[@]} -eq 0 ]]; then
  SOURCES=("packages/repo" "apk-artifacts")
fi

if [[ -z "$OUTPUT_DIR" ]] || [[ "$OUTPUT_DIR" == "/" ]] || [[ "$OUTPUT_DIR" == "." ]]; then
  echo "unsafe output directory: ${OUTPUT_DIR}" >&2
  exit 2
fi

if [[ ! "$KEY_NAME" =~ ^[A-Za-z0-9._-]+$ ]] || [[ "$KEY_NAME" == *".."* ]]; then
  echo "unsafe key name: ${KEY_NAME}" >&2
  exit 2
fi
if [[ "$KEY_NAME" != *.rsa ]]; then
  echo "key name must end with .rsa so signatures match the published .rsa.pub key: ${KEY_NAME}" >&2
  exit 2
fi

detect_arch() {
  local path="$1" parent
  parent=$(basename "$(dirname "$path")")
  case "$parent" in
    x86_64|aarch64|armv7|armhf|ppc64le|s390x|riscv64)
      printf '%s\n' "$parent"
      return 0
      ;;
  esac
  return 1
}

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required tool not found: $1" >&2
    exit 1
  fi
}

mapfile -t APKS < <(
  for source in "${SOURCES[@]}"; do
    [[ -d "$source" ]] || continue
    find "$source" -type f -name '*.apk' -print
  done | sort -u
)

rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

if [[ ${#APKS[@]} -eq 0 ]]; then
  cat > "$OUTPUT_DIR/.no-apks-found" <<EOF
No APK files were found in: ${SOURCES[*]}
Repository assembly skipped without failing the workflow.
EOF
  echo "No APK files found; wrote ${OUTPUT_DIR}/.no-apks-found"
  exit 0
fi

declare -A DESTINATIONS=()
for apk_file in "${APKS[@]}"; do
  arch=$(detect_arch "$apk_file") || {
    echo "could not determine APK architecture from parent directory: ${apk_file}" >&2
    echo "expected the APK to live directly under an arch directory such as x86_64 or aarch64" >&2
    exit 1
  }
  dest_key="${arch}/$(basename "$apk_file")"
  if [[ -n "${DESTINATIONS[$dest_key]:-}" ]]; then
    echo "duplicate APK destination ${dest_key}:" >&2
    echo "  ${DESTINATIONS[$dest_key]}" >&2
    echo "  ${apk_file}" >&2
    exit 1
  fi
  DESTINATIONS[$dest_key]="$apk_file"
done

require_tool apk
if [[ -n "${APK_REPOSITORY_PRIVATE_KEY:-}" ]]; then
  require_tool abuild-sign
  require_tool openssl
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

for apk_file in "${APKS[@]}"; do
  arch=$(detect_arch "$apk_file")
  mkdir -p "$OUTPUT_DIR/$arch"
  cp "$apk_file" "$OUTPUT_DIR/$arch/"
done

if [[ -n "${APK_REPOSITORY_PRIVATE_KEY:-}" ]]; then
  private_key="$tmpdir/$KEY_NAME"
  public_key="$tmpdir/$KEY_NAME.pub"
  printf '%s\n' "$APK_REPOSITORY_PRIVATE_KEY" > "$private_key"
  chmod 600 "$private_key"
  openssl rsa -in "$private_key" -pubout -out "$public_key"
  cp "$public_key" "$OUTPUT_DIR/$KEY_NAME.pub"
  echo "Published APK repository public key: ${OUTPUT_DIR}/${KEY_NAME}.pub"
else
  private_key=""
  echo "APK_REPOSITORY_PRIVATE_KEY not set; APKINDEX files will be unsigned" >&2
fi

for arch_dir in "$OUTPUT_DIR"/*; do
  [[ -d "$arch_dir" ]] || continue
  shopt -s nullglob
  arch_apks=("$arch_dir"/*.apk)
  shopt -u nullglob
  [[ ${#arch_apks[@]} -gt 0 ]] || continue

  (
    cd "$arch_dir"
    apk index --output APKINDEX.tar.gz ./*.apk
  )

  if [[ -n "$private_key" ]]; then
    abuild-sign -k "$private_key" "$arch_dir/APKINDEX.tar.gz"
  fi
  echo "Assembled ${arch_dir#"${OUTPUT_DIR}"/}/APKINDEX.tar.gz (${#arch_apks[@]} packages)"
done
