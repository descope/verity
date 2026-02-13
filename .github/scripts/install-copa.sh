#!/bin/bash
set -euo pipefail

echo "Installing Copa..."

# Use GITHUB_TOKEN when available to avoid API rate limits on Actions runners.
AUTH_HEADER=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
  AUTH_HEADER=(-H "Authorization: token ${GITHUB_TOKEN}")
fi

COPA_VERSION=$(curl -sfS "${AUTH_HEADER[@]}" https://api.github.com/repos/project-copacetic/copacetic/releases/latest | jq -r .tag_name | sed 's/^v//')

if [ -z "$COPA_VERSION" ] || [ "$COPA_VERSION" = "null" ]; then
  echo "::error::Failed to determine latest Copa version (API rate limited?)"
  exit 1
fi

echo "Latest Copa version: ${COPA_VERSION}"

curl -fsSL -o copa.tar.gz "https://github.com/project-copacetic/copacetic/releases/download/v${COPA_VERSION}/copa_${COPA_VERSION}_linux_amd64.tar.gz"
tar -xzf copa.tar.gz copa
sudo mv copa /usr/local/bin/
rm copa.tar.gz

copa --version
echo "Copa installed successfully"
