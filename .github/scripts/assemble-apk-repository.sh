#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: assemble-apk-repository.sh [--output DIR] [--key-name NAME] [--public-key FILE] [SOURCE_DIR ...]

Collects .apk files from SOURCE_DIRs, lays them out as a static APK repository,
builds APKINDEX.tar.gz per architecture, and signs each index when
APK_REPOSITORY_PRIVATE_KEY is set.

Defaults:
  output: site/dist/apk
  sources: packages/repo apk-artifacts
  key name: verity.rsa
  public key: keys/apk/verity.rsa.pub

Guarded behavior: if no .apk files are found, writes .no-apks-found and exits 0.

Preservation: any files in OUTPUT_DIR that this script does not own
(e.g. index.html, index.md from a prior Astro build) are left untouched.
Only managed artifacts (.no-apks-found, arch directories, the public key,
and APKINDEX.tar.gz files) are removed at the start of each run.
USAGE
}

# Architectures the script writes to OUTPUT_DIR. Single source of truth —
# both `detect_arch` (which classifies incoming .apk files) and the
# pre-assembly cleanup loop iterate over this list, so adding a new arch
# only requires editing one place.
SUPPORTED_ARCHES=(x86_64 aarch64 armv7 armhf ppc64le s390x riscv64)

OUTPUT_DIR="site/dist/apk"
KEY_NAME="verity.rsa"
PUBLIC_KEY_PATH=""
REPOSITORY_FORMAT_VERSION="1"
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
    --public-key)
      [[ $# -ge 2 ]] || { echo "missing value for $1" >&2; exit 2; }
      PUBLIC_KEY_PATH="$2"
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

has_parent_traversal() {
  local path="$1" segment
  IFS='/' read -ra segments <<< "$path"
  for segment in "${segments[@]}"; do
    [[ "$segment" == ".." ]] && return 0
  done
  return 1
}

if [[ -z "$OUTPUT_DIR" ]] || [[ "$OUTPUT_DIR" == "/" ]] || [[ "$OUTPUT_DIR" == "." ]] || has_parent_traversal "$OUTPUT_DIR"; then
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
if [[ -z "$PUBLIC_KEY_PATH" ]]; then
  PUBLIC_KEY_PATH="keys/apk/${KEY_NAME}.pub"
fi

detect_arch() {
  local path="$1" parent arch
  parent=$(basename "$(dirname "$path")")
  for arch in "${SUPPORTED_ARCHES[@]}"; do
    if [[ "$parent" == "$arch" ]]; then
      printf '%s\n' "$arch"
      return 0
    fi
  done
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

# Clean only artifacts this script manages. Preserve any other files (notably
# index.html and index.md emitted by the Astro build) so the published page
# still explains the repository even when no .apk files were produced.
mkdir -p "$OUTPUT_DIR"
rm -f "${OUTPUT_DIR:?}/.no-apks-found"
rm -f "${OUTPUT_DIR:?}/${KEY_NAME}.pub"
rm -f "${OUTPUT_DIR:?}/repository-format"
for arch in "${SUPPORTED_ARCHES[@]}"; do
  rm -rf "${OUTPUT_DIR:?}/${arch}"
done

if [[ ${#APKS[@]} -eq 0 ]]; then
  cat > "$OUTPUT_DIR/.no-apks-found" <<EOF
No APK files were found in: ${SOURCES[*]}
Repository assembly skipped without failing the workflow.
EOF
  echo "No APK files found; wrote ${OUTPUT_DIR}/.no-apks-found"
  exit 0
fi

if [[ -n "${APK_REPOSITORY_PRIVATE_KEY:-}" ]]; then
  require_tool abuild-sign
  require_tool cmp
  require_tool melange
  require_tool openssl
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

if [[ -n "${APK_REPOSITORY_PRIVATE_KEY:-}" ]]; then
  private_input="$tmpdir/private-key.pem"
  mkdir -p "$tmpdir/signing"
  private_key="$tmpdir/signing/$KEY_NAME"
  public_key="$OUTPUT_DIR/$KEY_NAME.pub"
  printf '%s\n' "$APK_REPOSITORY_PRIVATE_KEY" > "$private_input"
  chmod 600 "$private_input"
  [[ -f "$PUBLIC_KEY_PATH" ]] || { echo "APK repository public key not found: $PUBLIC_KEY_PATH" >&2; exit 1; }
  openssl pkey -in "$private_input" -pubout -outform DER > "$tmpdir/private-public.der"
  openssl pkey -pubin -in "$PUBLIC_KEY_PATH" -outform DER > "$tmpdir/committed-public.der"
  if ! cmp -s "$tmpdir/private-public.der" "$tmpdir/committed-public.der"; then
    echo "APK_REPOSITORY_PRIVATE_KEY does not match $PUBLIC_KEY_PATH" >&2
    exit 1
  fi
  openssl rsa -in "$private_input" -traditional -out "$private_key" >/dev/null 2>&1
  chmod 600 "$private_key"
  cp "$PUBLIC_KEY_PATH" "$public_key"
  printf '%s\n' "$REPOSITORY_FORMAT_VERSION" > "$OUTPUT_DIR/repository-format"
  echo "Published APK repository public key: $public_key"
else
  private_key=""
  echo "APK_REPOSITORY_PRIVATE_KEY not set; APKINDEX files will be unsigned" >&2
fi

declare -A DESTINATIONS=()
for apk_file in "${APKS[@]}"; do
  arch=$(detect_arch "$apk_file") || {
    echo "could not determine APK architecture from parent directory: ${apk_file}" >&2
    echo "expected the APK to live directly under an arch directory such as x86_64 or aarch64" >&2
    exit 1
  }
  dest_key="${arch}/$(basename "$apk_file")"
  destination="$OUTPUT_DIR/$dest_key"
  candidate="$tmpdir/$(printf '%s' "$dest_key" | tr '/' '-')"
  cp "$apk_file" "$candidate"
  if [[ -n "$private_key" ]]; then
    melange sign --signing-key "$private_key" "$candidate"
  fi
  if [[ -f "$destination" ]]; then
    if ! cmp -s "$candidate" "$destination"; then
      echo "duplicate APK destination ${dest_key}:" >&2
      echo "  ${DESTINATIONS[$dest_key]}" >&2
      echo "  ${apk_file}" >&2
      exit 1
    fi
    echo "Skipped byte-identical duplicate APK: $dest_key"
    continue
  fi
  mkdir -p "$OUTPUT_DIR/$arch"
  mv "$candidate" "$destination"
  DESTINATIONS[$dest_key]="$apk_file"
done

require_tool apk
repository_root=$(realpath "$OUTPUT_DIR")
for arch_dir in "$OUTPUT_DIR"/*; do
  [[ -d "$arch_dir" ]] || continue
  shopt -s nullglob
  arch_apks=("$arch_dir"/*.apk)
  shopt -u nullglob
  [[ ${#arch_apks[@]} -gt 0 ]] || continue

  (
    cd "$arch_dir"
    if [[ -n "$private_key" ]]; then
      apk index --keys-dir "$repository_root" --output APKINDEX.tar.gz ./*.apk
    else
      apk index --allow-untrusted --output APKINDEX.tar.gz ./*.apk
    fi
  )

  if [[ -n "$private_key" ]]; then
    abuild-sign -t RSA256 -k "$private_key" -p "$public_key" "$arch_dir/APKINDEX.tar.gz"
  fi
  echo "Assembled ${arch_dir#"${OUTPUT_DIR}"/}/APKINDEX.tar.gz (${#arch_apks[@]} packages)"
done
