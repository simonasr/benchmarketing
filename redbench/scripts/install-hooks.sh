#!/bin/sh

# Get the repository root directory
REPO_ROOT=$(git rev-parse --show-toplevel)

# Create hooks directory if it doesn't exist
mkdir -p "$REPO_ROOT/.git/hooks"

# Copy the pre-commit hook
cp "$REPO_ROOT/redbench/scripts/pre-commit" "$REPO_ROOT/.git/hooks/pre-commit"

# Make it executable
chmod +x "$REPO_ROOT/.git/hooks/pre-commit"

echo "Git hooks installed successfully!"