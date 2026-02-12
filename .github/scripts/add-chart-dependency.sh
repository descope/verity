#!/bin/bash
set -euo pipefail

# Adds a new chart dependency to Chart.yaml and creates a PR.
# Expects environment variables: CHART_NAME, CHART_VERSION, CHART_REPOSITORY, ISSUE_NUMBER

: "${CHART_NAME:?CHART_NAME is required}"
: "${CHART_VERSION:?CHART_VERSION is required}"
: "${CHART_REPOSITORY:?CHART_REPOSITORY is required}"
: "${ISSUE_NUMBER:?ISSUE_NUMBER is required}"

# Check for duplicate
if yq e ".dependencies[] | select(.name == \"${CHART_NAME}\")" Chart.yaml | grep -q name; then
  echo "Chart ${CHART_NAME} already exists in Chart.yaml"
  gh issue comment "${ISSUE_NUMBER}" \
    --body "Chart **${CHART_NAME}** already exists in Chart.yaml. Closing as duplicate."
  gh issue close "${ISSUE_NUMBER}"
  exit 0
fi

# Add chart dependency
yq e ".dependencies += [{\"name\": \"${CHART_NAME}\", \"version\": \"${CHART_VERSION}\", \"repository\": \"${CHART_REPOSITORY}\"}]" -i Chart.yaml

# Create PR
BRANCH="add-chart/${CHART_NAME}"
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
