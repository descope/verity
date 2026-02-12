#!/bin/bash
set -euo pipefail

# Parses a GitHub issue form body and extracts fields.
# Expects ISSUE_BODY environment variable.
# Sets GITHUB_OUTPUT variables: name, version, repository.

: "${ISSUE_BODY:?ISSUE_BODY is required}"

get_field() {
  local label="$1"
  echo "${ISSUE_BODY}" | grep -A1 "### ${label}" | tail -1 | xargs
}

NAME=$(get_field "Chart name")
VERSION=$(get_field "Chart version")
REPOSITORY=$(get_field "Chart repository")

if [ -z "${NAME}" ] || [ -z "${VERSION}" ] || [ -z "${REPOSITORY}" ]; then
  echo "::error::Missing required fields in issue body"
  exit 1
fi

{
  echo "name=${NAME}"
  echo "version=${VERSION}"
  echo "repository=${REPOSITORY}"
} >> "$GITHUB_OUTPUT"

echo "Parsed: ${NAME}@${VERSION} from ${REPOSITORY}"
