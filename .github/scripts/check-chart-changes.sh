#!/bin/bash
set -euo pipefail

# Checks if there are changes in the charts directory.
# Sets GITHUB_OUTPUT variable 'changes' to 'true' or 'false'.

if git diff --quiet charts/; then
  echo "changes=false" >> "$GITHUB_OUTPUT"
  echo "No new patches needed"
else
  echo "changes=true" >> "$GITHUB_OUTPUT"
  echo "New patches available"
fi
