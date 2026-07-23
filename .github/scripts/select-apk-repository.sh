#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: select-apk-repository.sh CANDIDATE_DIR PREVIOUS_DIR OUTPUT_DIR" >&2
  exit 2
fi

candidate_dir="$1"
previous_dir="$2"
output_dir="$3"
supported_arches=(x86_64 aarch64 armv7 armhf ppc64le s390x riscv64)

[[ -d "$candidate_dir" ]] || { echo "candidate APK repository not found: $candidate_dir" >&2; exit 1; }
if [[ -z "$output_dir" ]] || [[ "$output_dir" == "/" ]] || [[ "$output_dir" == "." ]] || [[ "/$output_dir/" == *"/../"* ]]; then
  echo "unsafe output directory: $output_dir" >&2
  exit 2
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

repository_state_manifest() {
  local repository="$1" manifest="$2" arch
  : > "$manifest"
  [[ -d "$repository" ]] || return 0
  (
    cd "$repository"
    for arch in "${supported_arches[@]}"; do
      [[ -d "$arch" ]] || continue
      find "$arch" -maxdepth 1 -type f -name '*.apk' -print0
    done
    find . -maxdepth 1 -type f \( -name '*.rsa.pub' -o -name 'repository-format' \) -print0
  ) | sort -z | (
    cd "$repository"
    xargs -0 -r sha256sum
  ) > "$manifest"
}

repository_state_manifest "$candidate_dir" "$tmpdir/candidate.sha256"
repository_state_manifest "$previous_dir" "$tmpdir/previous.sha256"

selected_dir="$candidate_dir"
if [[ -d "$previous_dir" ]] && cmp -s "$tmpdir/candidate.sha256" "$tmpdir/previous.sha256"; then
  selected_dir="$previous_dir"
  echo "APK repository state unchanged; preserving previously published package and index bytes"
else
  echo "APK repository state changed; publishing the newly assembled repository"
fi

mkdir -p "$output_dir"
rm -f "${output_dir:?}/.no-apks-found"
rm -f "${output_dir:?}/repository-format"
find "$output_dir" -maxdepth 1 -type f -name '*.rsa.pub' -delete
for arch in "${supported_arches[@]}"; do
  rm -rf "${output_dir:?}/${arch}"
  if [[ -d "$selected_dir/$arch" ]]; then
    cp -a "$selected_dir/$arch" "$output_dir/"
  fi
done

shopt -s nullglob
public_keys=("$selected_dir"/*.rsa.pub)
shopt -u nullglob
if [[ ${#public_keys[@]} -eq 0 ]]; then
  echo "selected APK repository has no public key" >&2
  exit 1
fi
cp "${public_keys[@]}" "$output_dir/"
if [[ -f "$selected_dir/repository-format" ]]; then
  cp "$selected_dir/repository-format" "$output_dir/"
fi
