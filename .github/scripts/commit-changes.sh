#!/bin/bash
set -euo pipefail

CHARTS_DIR="${1:-charts}"

echo "Checking for changes in ${CHARTS_DIR}/..."

git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"

if git diff --quiet "${CHARTS_DIR}/" && git diff --quiet reports/ && git diff --quiet site/src/data/catalog.json 2>/dev/null && git diff --quiet values.yaml 2>/dev/null; then
  echo "No changes to commit"
  exit 0
fi

echo "Changes detected, committing..."
git add "${CHARTS_DIR}/"
# Also stage values.yaml if it was modified (merged chart images from discover)
if ! git diff --quiet values.yaml 2>/dev/null; then
  git add values.yaml
fi
# Also stage standalone image reports if they exist
if [ -d "reports" ]; then
  git add reports/
fi
# Also stage regenerated site catalog if it exists
if [ -f "site/src/data/catalog.json" ]; then
  git add site/src/data/catalog.json
fi
git commit -m "chore: patch images for updated chart versions

Automatically patched container images using Copa after chart version update.

Co-Authored-By: github-actions[bot] <github-actions[bot]@users.noreply.github.com>"

echo "Pushing changes..."
git push

echo "✅ Changes committed and pushed successfully"
