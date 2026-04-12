#!/bin/bash
set -euo pipefail

# Parses a GitHub issue form body and extracts image fields.
# Expects ISSUE_BODY environment variable.
# Sets GITHUB_OUTPUT variables: name, repository, tag, registry.

: "${ISSUE_BODY:?ISSUE_BODY is required}"

get_field() {
  local label="$1"
  printf '%s\n' "${ISSUE_BODY}" | sed -n "/### ${label}/,/### /p" | sed '1d;/^### /d;/^$/d' | head -1 | xargs
}

NAME=$(get_field "Image name")
REPOSITORY=$(get_field "Image repository")
TAG=$(get_field "Image tag")
REGISTRY=$(get_field "Image registry")

# Default registry to docker.io
if [ -z "${REGISTRY}" ]; then
  REGISTRY="docker.io"
fi

# Validate extracted fields against safe character sets
validate() {
  local label="$1" value="$2" pattern="$3"
  if [[ ! "$value" =~ $pattern ]]; then
    echo "::error::Invalid ${label}: '${value}'"
    exit 1
  fi
}

validate "Image name"       "$NAME"       '^[a-z][a-z0-9._-]{0,127}$'
validate "Image repository"  "$REPOSITORY" '^[a-z0-9][a-z0-9._/-]{0,255}$'
validate "Image tag"         "$TAG"        '^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$'
validate "Image registry"    "$REGISTRY"   '^(docker\.io|gcr\.io|mirror\.gcr\.io|ghcr\.io|quay\.io|mcr\.microsoft\.com|registry\.k8s\.io|public\.ecr\.aws)$'

if [ -z "${NAME}" ] || [ -z "${REPOSITORY}" ] || [ -z "${TAG}" ]; then
  echo "::error::Missing required fields in issue body"
  exit 1
fi

{
  echo "name=${NAME}"
  echo "repository=${REPOSITORY}"
  echo "tag=${TAG}"
  echo "registry=${REGISTRY}"
} >> "$GITHUB_OUTPUT"

echo "Parsed: ${NAME} → ${REGISTRY}/${REPOSITORY}:${TAG}"
