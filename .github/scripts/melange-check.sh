#!/bin/bash
set -euo pipefail
#
# Checks if an image type requires a melange build and outputs the
# melange configuration fields. Expects IMAGE and TYPE env vars
# (from the workflow matrix). Writes to GITHUB_OUTPUT.

: "${IMAGE:?IMAGE is required}"
: "${TYPE:?TYPE is required}"

image_yaml="images/${IMAGE}.yaml"

bespoke=$(yq -r ".types.\"${TYPE}\".melange.bespoke // \"\"" "$image_yaml")
upstream=$(yq -r ".types.\"${TYPE}\".melange.upstream // \"\"" "$image_yaml")
env_file=$(yq -r ".types.\"${TYPE}\".melange.env-file // \"\"" "$image_yaml")
build_option=$(yq -r ".types.\"${TYPE}\".melange.build-option // \"\"" "$image_yaml")

if [ -n "$bespoke" ] || [ -n "$upstream" ]; then
  needed=true
else
  needed=false
fi

for field_name in bespoke upstream; do
  field_val="${!field_name}"
  if [ -n "$field_val" ] && [[ ! "$field_val" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "melange.${field_name} contains unsafe characters: '${field_val}'" >&2
    exit 1
  fi
done
for field_name in env_file build_option; do
  field_val="${!field_name}"
  if [[ "$field_val" == *$'\n'* ]]; then
    echo "melange.${field_name//_/-} must be a scalar string, got multi-line value" >&2
    exit 1
  fi
done

{
  echo "needed=${needed}"
  echo "bespoke=${bespoke}"
  echo "upstream=${upstream}"
  echo "env_file=${env_file}"
  echo "build_option=${build_option}"
} >> "$GITHUB_OUTPUT"
