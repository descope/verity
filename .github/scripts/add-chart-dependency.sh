#!/bin/bash
set -euo pipefail

# Adds a new chart dependency to Chart.yaml and creates a PR.
# Expects environment variables: CHART_NAME, CHART_VERSION, CHART_REPOSITORY, ISSUE_NUMBER

: "${CHART_NAME:?CHART_NAME is required}"
: "${CHART_VERSION:?CHART_VERSION is required}"
: "${CHART_REPOSITORY:?CHART_REPOSITORY is required}"
: "${ISSUE_NUMBER:?ISSUE_NUMBER is required}"

# Normalize repository URL: prepend https:// if no protocol specified
if [[ ! "${CHART_REPOSITORY}" =~ ^(oci|https?):// ]]; then
  CHART_REPOSITORY="https://${CHART_REPOSITORY}"
  echo "Normalized repository URL to: ${CHART_REPOSITORY}"
fi

# Check for duplicate
if yq e ".dependencies[] | select(.name == strenv(CHART_NAME))" Chart.yaml | grep -q name; then
  echo "Chart ${CHART_NAME} already exists in Chart.yaml"
  gh issue comment "${ISSUE_NUMBER}" \
    --body "Chart **${CHART_NAME}** already exists in Chart.yaml. Closing as duplicate."
  gh issue close "${ISSUE_NUMBER}"
  exit 0
fi

# Add chart dependency using env vars to avoid injection
export CHART_NAME CHART_VERSION CHART_REPOSITORY
yq e '.dependencies += [{"name": strenv(CHART_NAME), "version": strenv(CHART_VERSION), "repository": strenv(CHART_REPOSITORY)}]' -i Chart.yaml

# Sanitize branch name
SAFE_NAME=$(echo "${CHART_NAME}" | tr -cs '[:alnum:]-' '-' | sed 's/^-//;s/-$//')
BRANCH="add-chart/${SAFE_NAME}"

git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"
git checkout -b "${BRANCH}"
git add Chart.yaml
git commit -m "feat: add ${CHART_NAME} chart dependency"
git push -u origin "${BRANCH}"
gh pr create \
  --title "Add ${CHART_NAME} chart" \
  --body "Adds ${CHART_NAME}@${CHART_VERSION} from \`${CHART_REPOSITORY}\`.

Closes #${ISSUE_NUMBER}" \
  --label new-chart
