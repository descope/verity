#!/bin/bash
# Setup script to install git hooks

set -e

echo "Installing git hooks..."

# Copy pre-commit hook
cp .githooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit

echo "✓ Git hooks installed successfully!"
echo ""
echo "The pre-commit hook will run 'make test' and 'make quality' before each commit."
echo "To skip the hook temporarily, use: git commit --no-verify"
