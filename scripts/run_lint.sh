#!/bin/bash
# Script to run linters on all components and the Go SDK
# This script is used by the 'make lint' target

set -e


echo ""
echo "--------------------------------------------"
echo "   📝 Running linters on all components  "
echo "--------------------------------------------"

# Check and lint components
for dir in ./components/*; do
  if [ -d "$dir" ]; then
    component=$(basename "$dir")
    echo "Checking for Go files in $dir..."
    
    # Check if directory contains Go files
    if [ -z "$(find "$dir" -name "*.go" -type f -print -quit)" ]; then
      echo "No Go files found in $dir, skipping linting"
      continue
    fi
    
    echo "Linting in $dir..."
    
    # Check if golangci-lint is installed
    if ! command -v golangci-lint &> /dev/null; then
      echo "Installing golangci-lint..."
      curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
    fi
    
    # Run linters
    cd "$dir"
    echo ""
    echo "--------------------------"
    echo "   📝 Running linters  "
    echo "--------------------------"
    
    # Run with error handling for config issues
    if ! golangci-lint run ./... 2>/tmp/golangci_error; then
      error=$(cat /tmp/golangci_error)
      if [[ $error == *"unsupported version of the configuration"* ]]; then
        echo "[warning] Skipping due to config version issue"
        echo "See https://golangci-lint.run/product/migration-guide for migration instructions"
      else
        echo "[error] Linting failed ❌"
        echo "$error"
        exit 1
      fi
    fi
    
    cd - > /dev/null
    echo "[ok] Linting completed successfully ✔️"
  fi
done

# Lint Go SDK
echo "Linting Go SDK..."
cd ./sdks/go
echo ""
echo "--------------------------"
echo "   📝 Running linters  "
echo "--------------------------"

# Run with error handling for config issues
if ! golangci-lint run ./... 2>/tmp/golangci_error; then
  error=$(cat /tmp/golangci_error)
  if [[ $error == *"unsupported version of the configuration"* ]]; then
    echo "[warning] Skipping due to config version issue"
    echo "See https://golangci-lint.run/product/migration-guide for migration instructions"
  else
    echo "[error] Linting failed ❌"
    echo "$error"
    exit 1
  fi
fi

cd - > /dev/null
echo "[ok] Linting completed successfully ✔️"

echo "[ok] Linting completed successfully ✔️"
