#!/bin/bash
set -euo pipefail

# Parses a GitHub issue form body and extracts image fields.
# Expects ISSUE_BODY environment variable.
# Sets GITHUB_OUTPUT variables: name, repository, tag, registry.

: "${ISSUE_BODY:?ISSUE_BODY is required}"

get_field() {
  local label="$1"
  echo "${ISSUE_BODY}" | sed -n "/### ${label}/,/### /p" | sed '1d;/^### /d' | grep -v '^$' | head -1 | xargs
}

NAME=$(get_field "Image name")
REPOSITORY=$(get_field "Image repository")
TAG=$(get_field "Image tag")
REGISTRY=$(get_field "Image registry")

if [ -z "${NAME}" ] || [ -z "${REPOSITORY}" ] || [ -z "${TAG}" ]; then
  echo "::error::Missing required fields in issue body"
  exit 1
fi

# Default registry to docker.io
if [ -z "${REGISTRY}" ]; then
  REGISTRY="docker.io"
fi

{
  echo "name=${NAME}"
  echo "repository=${REPOSITORY}"
  echo "tag=${TAG}"
  echo "registry=${REGISTRY}"
} >> "$GITHUB_OUTPUT"

echo "Parsed: ${NAME} → ${REGISTRY}/${REPOSITORY}:${TAG}"
