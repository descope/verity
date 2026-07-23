#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: validate-apk-repository.sh [--require-signature] [--verify-crypto] REPO_DIR

Validates a static APK repository layout:
  REPO_DIR/<arch>/*.apk
  REPO_DIR/<arch>/APKINDEX.tar.gz
  REPO_DIR/*.rsa.pub when signatures are required

If REPO_DIR/.no-apks-found exists and no APKs are present, validation passes.

--verify-crypto additionally verifies every package signature and performs a
fresh-client apk update for each architecture. It implies --require-signature.
USAGE
}

REQUIRE_SIGNATURE=false
VERIFY_CRYPTO=false
REPO_DIR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --require-signature)
      REQUIRE_SIGNATURE=true
      shift
      ;;
    --verify-crypto)
      REQUIRE_SIGNATURE=true
      VERIFY_CRYPTO=true
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
if [[ "$VERIFY_CRYPTO" == "true" ]] && ! command -v apk >/dev/null 2>&1; then
  echo "apk is required for cryptographic repository verification" >&2
  exit 1
fi
if [[ "$VERIFY_CRYPTO" == "true" ]] && ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required for negative trust verification" >&2
  exit 1
fi

is_supported_arch() {
  case "$1" in
    x86_64|aarch64|armv7|armhf|ppc64le|s390x|riscv64) return 0 ;;
    *) return 1 ;;
  esac
}

mapfile -t ROOT_APKS < <(find "$REPO_DIR" -maxdepth 1 -type f -name '*.apk' -print | sort)
if [[ ${#ROOT_APKS[@]} -gt 0 ]]; then
  echo "APK files must live under architecture directories, not repository root:" >&2
  printf '  %s\n' "${ROOT_APKS[@]}" >&2
  exit 1
fi

mapfile -t DEEP_APKS < <(find "$REPO_DIR" -mindepth 3 -type f -name '*.apk' -print | sort)
if [[ ${#DEEP_APKS[@]} -gt 0 ]]; then
  echo "APK files must live directly under architecture directories:" >&2
  printf '  %s\n' "${DEEP_APKS[@]}" >&2
  exit 1
fi

mapfile -t APKS < <(find "$REPO_DIR" -mindepth 2 -maxdepth 2 -type f -name '*.apk' -print | sort)
if [[ ${#APKS[@]} -eq 0 ]]; then
  if [[ -f "$REPO_DIR/.no-apks-found" ]]; then
    echo "No APK files present; guarded empty repository marker found"
    exit 0
  fi
  echo "no APK files found and .no-apks-found marker is missing" >&2
  exit 1
fi

if [[ "$VERIFY_CRYPTO" == "true" ]]; then
  for required_arch in x86_64 aarch64; do
    if ! find "$REPO_DIR/$required_arch" -maxdepth 1 -type f -name '*.apk' -print -quit 2>/dev/null | grep -q .; then
      echo "required architecture has no APK packages: $required_arch" >&2
      exit 1
    fi
  done
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
  arch=$(basename "$arch_dir")
  shopt -s nullglob
  arch_apks=("$arch_dir"/*.apk)
  shopt -u nullglob
  [[ ${#arch_apks[@]} -gt 0 ]] || continue

  if ! is_supported_arch "$arch"; then
    echo "unsupported architecture directory containing APKs: $arch" >&2
    status=1
    continue
  fi

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

  if [[ "$REQUIRE_SIGNATURE" == "true" ]]; then
    mapfile -t SIGNATURES < <(tar -tzf "$index" | grep -E '(^|/)\.SIGN\.RSA256\..*\.rsa\.pub$' | sed 's#^.*/##')
    if [[ ${#SIGNATURES[@]} -eq 0 ]]; then
      echo "missing RSA256 signature entry in $index" >&2
      status=1
      continue
    fi
    for signature in "${SIGNATURES[@]}"; do
      key_name="${signature#.SIGN.RSA256.}"
      if [[ ! -f "$REPO_DIR/$key_name" ]]; then
        echo "signature ${signature} in $index has no matching root public key: $key_name" >&2
        status=1
      fi
    done
  fi

  if [[ "$VERIFY_CRYPTO" == "true" ]]; then
    for apk_file in "${arch_apks[@]}"; do
      if ! apk verify --keys-dir "$REPO_DIR" "$apk_file"; then
        echo "APK signature verification failed: $apk_file" >&2
        status=1
      fi
    done

    client_root=$(mktemp -d)
    repositories_file="$client_root/repositories"
    printf 'file:///repo\n' > "$repositories_file"
    apk --root "$client_root/root" --arch "$arch" \
      --repositories-file /dev/null add --initdb >/dev/null
    mkdir -p "$client_root/root/etc/apk/keys" "$client_root/root/repo/$arch"
    cp "$REPO_DIR"/*.rsa.pub "$client_root/root/etc/apk/keys/"
    cp "$index" "$client_root/root/repo/$arch/APKINDEX.tar.gz"
    if ! update_output=$(apk --root "$client_root/root" --arch "$arch" \
      --keys-dir "$client_root/root/etc/apk/keys" --repositories-file "$repositories_file" \
      --no-cache update 2>&1); then
      printf '%s\n' "$update_output" >&2
      echo "fresh-client repository update failed: $arch" >&2
      status=1
    elif grep -Eq 'BAD signature|UNTRUSTED signature|No such file or directory' <<< "$update_output" || \
      ! grep -Eq '(^| )[1-9][0-9]* distinct packages available' <<< "$update_output"; then
      printf '%s\n' "$update_output" >&2
      echo "fresh-client repository update did not load trusted packages: $arch" >&2
      status=1
    fi

    wrong_keys="$client_root/wrong-keys"
    mkdir -p "$wrong_keys"
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 \
      -out "$client_root/wrong.rsa" >/dev/null 2>&1
    openssl pkey -in "$client_root/wrong.rsa" -pubout \
      -out "$wrong_keys/${SIGNATURES[0]#.SIGN.RSA256.}" >/dev/null 2>&1
    if apk verify --keys-dir "$wrong_keys" "${arch_apks[0]}" >/dev/null 2>&1; then
      echo "wrong key unexpectedly verified an APK: ${arch_apks[0]}" >&2
      status=1
    fi
    apk --root "$client_root/wrong-root" --arch "$arch" \
      --repositories-file /dev/null add --initdb >/dev/null
    mkdir -p "$client_root/wrong-root/etc/apk/keys" "$client_root/wrong-root/repo/$arch"
    cp "$wrong_keys"/*.rsa.pub "$client_root/wrong-root/etc/apk/keys/"
    cp "$index" "$client_root/wrong-root/repo/$arch/APKINDEX.tar.gz"
    wrong_update_output=$(apk --root "$client_root/wrong-root" --arch "$arch" \
      --keys-dir "$client_root/wrong-root/etc/apk/keys" --repositories-file "$repositories_file" \
      --no-cache update 2>&1 || true)
    if ! grep -Eq 'BAD signature|UNTRUSTED signature' <<< "$wrong_update_output"; then
      echo "wrong key unexpectedly verified repository index: $arch" >&2
      status=1
    fi
    rm -rf "$client_root"
  fi
done

if [[ $status -ne 0 ]]; then
  exit "$status"
fi

echo "APK repository layout valid: $REPO_DIR (${#APKS[@]} packages)"
