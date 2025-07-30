#!/bin/sh

# Script to install all required tools for development and pre-commit checks

# Exit on error
set -e

echo "Installing Go development tools..."

# Install staticcheck for static analysis and dead code detection
echo "Installing staticcheck..."
go install honnef.co/go/tools/cmd/staticcheck@latest

# Install golangci-lint for comprehensive linting
echo "Installing golangci-lint..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install errcheck for error checking
echo "Installing errcheck..."
go install github.com/kisielk/errcheck@latest

# Install goimports for import formatting
echo "Installing goimports..."
go install golang.org/x/tools/cmd/goimports@latest

# Install unparam to find unused function parameters
echo "Installing unparam..."
go install mvdan.cc/unparam@latest

# Install go-critic for additional code checks
echo "Installing go-critic..."
go install github.com/go-critic/go-critic/cmd/gocritic@latest

echo ""
echo "All tools installed successfully!"

# Also add the Go version-specific bin directory to PATH
echo "Also check if you need to add Go version-specific bin directories:"
echo "  export PATH=\\$PATH:\\$HOME/go/1.24.3/bin  # Adjust version as needed"
echo "Make sure \\$HOME/go/bin is in your PATH."
echo ""
echo "You can add it by adding this to your shell profile:"
echo "  export PATH=\\$PATH:\\$HOME/go/bin"