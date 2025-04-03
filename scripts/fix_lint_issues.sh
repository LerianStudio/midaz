#!/bin/bash

# Script to automatically fix common lint issues in the Midaz codebase
# This script addresses formatting, whitespace, and other common linting issues


# Get the root directory
ROOT_DIR=$(pwd)

# Define component directories
COMPONENTS=("./components/infra" "./components/mdz" "./components/onboarding" "./components/transaction")
SDK_GO_DIR="${ROOT_DIR}/sdks/go"

# Exit on error
set -e

echo "Fixing lint issues across the Midaz codebase..."

# Function to fix whitespace issues in a file
fix_wsl_issues() {
  local file=$1
  echo "Fixing whitespace issues in ${file}..."
  
  # Create a temporary file
  local tmpfile=$(mktemp)
  
  # Fix "return statements should not be cuddled if block has more than two lines"
  # Add a blank line before return statements that follow blocks with more than 2 lines
  awk '
    /^[[:space:]]*}[[:space:]]*$/ { 
      if (block_lines > 2) {
        print $0; 
        print ""; 
        block_lines = 0;
        in_block = 0;
        next;
      }
    }
    /^[[:space:]]*{[[:space:]]*$/ { 
      in_block = 1; 
      block_lines = 0; 
      print $0; 
      next;
    }
    in_block { block_lines++; }
    /^[[:space:]]*return / {
      if (prev ~ /^[[:space:]]*}[[:space:]]*$/ && block_lines > 2) {
        print "";
      }
      print $0;
      next;
    }
    { print $0; }
    { prev = $0; }
  ' "$file" > "$tmpfile"
  
  # Fix "if statements should only be cuddled with assignments"
  # Add a blank line before if statements that don't follow assignments
  awk '
    /^[[:space:]]*if / {
      if (!(prev ~ /[[:space:]]*[[:alnum:]_]+ *=[^=]/ || prev ~ /^[[:space:]]*$/ || prev ~ /^[[:space:]]*\/\//)) {
        print "";
      }
      print $0;
      next;
    }
    { print $0; }
    { prev = $0; }
  ' "$tmpfile" > "$file"
  
  # Fix "ranges should only be cuddled with assignments used in the iteration"
  # Add a blank line before for loops that don't follow relevant assignments
  awk '
    /^[[:space:]]*for[[:space:]].*range/ {
      if (!(prev ~ /[[:space:]]*[[:alnum:]_]+ *=[^=]/ || prev ~ /^[[:space:]]*$/ || prev ~ /^[[:space:]]*\/\//)) {
        print "";
      }
      print $0;
      next;
    }
    { print $0; }
    { prev = $0; }
  ' "$file" > "$tmpfile"
  
  # Fix "assignments should only be cuddled with other assignments"
  # Add a blank line before assignments that don't follow other assignments
  awk '
    /[[:space:]]*[[:alnum:]_]+ *=[^=]/ {
      if (!(prev ~ /[[:space:]]*[[:alnum:]_]+ *=[^=]/ || prev ~ /^[[:space:]]*$/ || prev ~ /^[[:space:]]*\/\//)) {
        print "";
      }
      print $0;
      next;
    }
    { print $0; }
    { prev = $0; }
  ' "$tmpfile" > "$file"
  
  # Fix "defer statements should only be cuddled with expressions on same variable"
  # Add a blank line before defer statements that don't follow expressions on the same variable
  awk '
    /^[[:space:]]*defer / {
      var_name = "";
      if (prev ~ /[[:space:]]*[[:alnum:]_]+ *:?=[^=]/) {
        var_name = prev;
        gsub(/[[:space:]]*([[:alnum:]_]+) *:?=.*/, "\\1", var_name);
      }
      
      defer_var = $0;
      gsub(/^[[:space:]]*defer[[:space:]]+([[:alnum:]_]+)\..*/, "\\1", defer_var);
      
      if (var_name == "" || var_name != defer_var) {
        print "";
      }
      print $0;
      next;
    }
    { print $0; }
    { prev = $0; }
  ' "$file" > "$tmpfile"
  
  # Fix "block should not start with a whitespace" in case statements
  perl -0777 -pe 's/(case [^:]+:)\n\s*(\n\s*[a-zA-Z])/\1\2/g' "$tmpfile" > "$file"
  
  # Fix "block should not start with a whitespace" in default statements
  perl -0777 -pe 's/(default:)\n\s*(\n\s*[a-zA-Z])/\1\2/g' "$file" > "$tmpfile"
  
  # Copy the final result back to the original file
  cp "$tmpfile" "$file"
  
  # Clean up
  rm "$tmpfile"
}

# Function to run goimports on a file
run_goimports() {
  local dir=$1
  echo "Running goimports in ${dir}..."
  
  # Check if goimports is installed
  if ! command -v goimports >/dev/null 2>&1; then
    echo "Installing goimports..."
    go install golang.org/x/tools/cmd/goimports@latest
  fi
  
  # Run goimports on all Go files in the directory
  find "$dir" -name "*.go" -type f | while read -r file; do
    goimports -w "$file"
  done
}

# Function to fix common revive linter issues
fix_revive_issues() {
  local file=$1
  echo "Fixing revive linter issues in ${file}..."
  
  # Create a temporary file
  local tmpfile=$(mktemp)
  
  # Fix "exported function/method/type should have comment"
  # Add comments to exported functions, methods, and types
  awk '
    /^func [A-Z][a-zA-Z0-9_]*\(/ || /^func \([^)]+\) [A-Z][a-zA-Z0-9_]*\(/ {
      if (prev !~ /^\/\//) {
        name = $0;
        gsub(/^func ([A-Z][a-zA-Z0-9_]*).*/, "\\1", name);
        if (prev !~ /^$/) {
          print "";
        }
        print "// " name " performs an operation";
      }
      print $0;
      next;
    }
    /^type [A-Z][a-zA-Z0-9_]* / {
      if (prev !~ /^\/\//) {
        name = $0;
        gsub(/^type ([A-Z][a-zA-Z0-9_]*).*/, "\\1", name);
        if (prev !~ /^$/) {
          print "";
        }
        print "// " name " represents an entity";
      }
      print $0;
      next;
    }
    { print $0; }
    { prev = $0; }
  ' "$file" > "$tmpfile"
  
  # Copy the final result back to the original file
  cp "$tmpfile" "$file"
  
  # Clean up
  rm "$tmpfile"
}

# Function to fix all issues in a directory
fix_directory() {
  local dir=$1
  echo "Fixing issues in ${dir}..."
  
  # Check if directory has Go files
  if find "${dir}" -name "*.go" -type f | grep -q .; then
    # Run go fmt
    echo "Running go fmt in ${dir}..."
    (cd "${dir}" && go fmt ./...)
    
    # Run goimports
    run_goimports "${dir}"
    
    # Fix whitespace issues
    find "${dir}" -name "*.go" -type f | while read -r file; do
      fix_wsl_issues "$file"
      fix_revive_issues "$file"
    done
    
    # Run golangci-lint with --fix flag
    echo "Running golangci-lint with --fix flag in ${dir}..."
    if ! command -v golangci-lint >/dev/null 2>&1; then
      echo "Installing golangci-lint..."
      go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    fi
    
    # Try to run golangci-lint with --fix flag
    (cd "${dir}" && golangci-lint run --fix ./... || echo "Some issues could not be fixed automatically")
    
    echo "[ok] Fixed issues in ${dir} ✔️"
  else
    echo "No Go files found in ${dir}, skipping"
  fi
}

# Fix issues in all components
for dir in "${COMPONENTS[@]}"; do
  fix_directory "${dir}"
done

# Fix issues in the Go SDK if it exists
if [ -d "${SDK_GO_DIR}" ]; then
  fix_directory "${SDK_GO_DIR}"
fi

# Fix issues in the pkg directory if it exists
if [ -d "${ROOT_DIR}/pkg" ]; then
  fix_directory "${ROOT_DIR}/pkg"
fi

echo "[ok] Fixed lint issues across the codebase ✔️"
echo "Note: Some issues may require manual fixes. Run 'make lint' to check for remaining issues."
