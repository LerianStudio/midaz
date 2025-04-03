#!/bin/bash
# Script to format code in all components and the Go SDK
# This script is used by the 'make format' target

set -e


echo ""
echo "--------------------------------------------"
echo "   🧹 Formatting code in all components  "
echo "--------------------------------------------"

# Format components
for dir in ./components/*; do
  if [ -d "$dir" ]; then
    component=$(basename "$dir")
    echo "Checking for Go files in $dir..."
    
    # Check if directory contains Go files
    if [ -z "$(find "$dir" -name "*.go" -type f -print -quit)" ]; then
      echo "No Go files found in $dir, skipping formatting"
      continue
    fi
    
    echo "Formatting code in $dir..."
    
    # Run gofmt
    cd "$dir"
    echo ""
    echo "--------------------------"
    echo "   🧹 Formatting code  "
    echo "--------------------------"
    
    # Find all Go files and format them
    find . -name "*.go" -type f | xargs gofmt -s -w
    
    # Run go imports if available
    if command -v goimports &> /dev/null; then
      find . -name "*.go" -type f | xargs goimports -w
    else
      echo "[warning] goimports not found, skipping import formatting"
    fi
    
    cd - > /dev/null
    echo "[ok] Formatting completed successfully ✔️"
  fi
done

# Format Go SDK
if [ -d "./sdks/go" ]; then
  echo "Formatting Go SDK..."
  cd ./sdks/go
  echo ""
  echo "--------------------------"
  echo "   🧹 Formatting code  "
  echo "--------------------------"
  
  # Find all Go files and format them
  find . -name "*.go" -type f | xargs gofmt -s -w
  
  # Run go imports if available
  if command -v goimports &> /dev/null; then
    find . -name "*.go" -type f | xargs goimports -w
  else
    echo "[warning] goimports not found, skipping import formatting"
  fi
  
  cd - > /dev/null
  echo "[ok] Formatting completed successfully ✔️"
fi

echo "[ok] All code formatting completed successfully ✔️"
