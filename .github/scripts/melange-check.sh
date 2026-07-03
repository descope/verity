#!/bin/bash
set -euo pipefail
#
# Checks if an image type requires a melange build and outputs the
# melange configuration fields. Expects IMAGE and TYPE env vars
# (from the workflow matrix). Writes to GITHUB_OUTPUT.

: "${IMAGE:?IMAGE is required}"
: "${TYPE:?TYPE is required}"

image_yaml="images/${IMAGE}.yaml"

raw_bespoke=$(yq -o=json ".types.\"${TYPE}\".melange.bespoke // []" "$image_yaml")
upstream=$(yq -r ".types.\"${TYPE}\".melange.upstream // \"\"" "$image_yaml")
env_file=$(yq -r ".types.\"${TYPE}\".melange.env-file // \"\"" "$image_yaml")
build_option=$(yq -r ".types.\"${TYPE}\".melange.build-option // \"\"" "$image_yaml")

bespoke_json=$(printf '%s' "$raw_bespoke" | jq -c 'if type == "array" then . elif . == null or . == "" then [] else [.] end')

if [ "$bespoke_json" != '[]' ] || [ -n "$upstream" ]; then
  needed=true
else
  needed=false
fi

while IFS= read -r bespoke; do
  if [ -n "$bespoke" ] && [[ ! "$bespoke" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "melange.bespoke contains unsafe characters: '${bespoke}'" >&2
    exit 1
  fi
done < <(printf '%s' "$bespoke_json" | jq -r '.[]')

if [ -n "$upstream" ] && [[ ! "$upstream" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "melange.upstream contains unsafe characters: '${upstream}'" >&2
  exit 1
fi

if [ -n "$upstream" ] && [ "$bespoke_json" != '[]' ]; then
  echo "melange block has both upstream and bespoke set — choose one" >&2
  exit 1
fi
for field_name in env_file build_option; do
  field_val="${!field_name}"
  if [[ "$field_val" == *$'\n'* ]]; then
    echo "melange.${field_name//_/-} must be a scalar string, got multi-line value" >&2
    exit 1
  fi
done

{
  echo "needed=${needed}"
  echo "bespoke_json=${bespoke_json}"
  echo "upstream=${upstream}"
  echo "env_file=${env_file}"
  echo "build_option=${build_option}"
} >> "$GITHUB_OUTPUT"
