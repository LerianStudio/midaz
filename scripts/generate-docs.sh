#!/bin/bash
set -euo pipefail

# Root directory of the repo
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Components to process
COMPONENTS=("onboarding" "transaction")

# Temporary log dir
LOG_DIR="${ROOT_DIR}/tmp/generate-docs-logs"
mkdir -p "${LOG_DIR}"

# Print a header with a nice box
print_header() {
  local text="$1"
  local length=${#text}
  local padding=$(( (60 - length) / 2 ))
  local line=$(printf '%*s' 60 | tr ' ' '-')
  
  echo -e "\n${line}"
  printf "%*s%s%*s\n" $padding "" "$text" $padding ""
  echo -e "${line}\n"
}

print_header "Generating Swagger API Documentation"

# Function to generate docs for a component
generate_component_docs() {
  local component=$1
  local component_dir="${ROOT_DIR}/components/${component}"
  local out_log="${LOG_DIR}/${component}.out"
  local err_log="${LOG_DIR}/${component}.err"
  
  # Capitalize first letter of component name
  component_display="$(tr '[:lower:]' '[:upper:]' <<< ${component:0:1})${component:1}"
  printf "  %-20s " "${component_display}"
  
  # Ensure the api directory exists
  mkdir -p "${component_dir}/api"
  
  # Change to the component directory and run the generate-docs target
  # We use a separate process to avoid directory change issues
  (
    cd "${component_dir}" || { 
      echo -e "❌ Could not access directory"
      echo "Failed to change to directory: ${component_dir}" > "${err_log}"
      return 1
    }
    
    # Run the make command directly
    if make generate-docs > "${out_log}" 2> "${err_log}"; then
      echo -e "✅ SUCCESS"
    else
      echo -e "❌ FAILED"
      echo "Failed to generate docs for ${component}" >> "${err_log}"
      return 1
    fi
  )
  
  # Verify that files were generated
  if [ ! -f "${component_dir}/api/swagger.json" ]; then
    echo -e "  ⚠️  Warning: swagger.json was not generated"
    echo "Warning: swagger.json was not generated for ${component}" >> "${err_log}"
  fi
}

# Generate docs for each component
for component in "${COMPONENTS[@]}"; do
  generate_component_docs "${component}"
done

# Summary of Swagger documentation generation
print_header "Documentation Generation Summary"

for component in "${COMPONENTS[@]}"; do
  out_log="${LOG_DIR}/${component}.out"
  err_log="${LOG_DIR}/${component}.err"
  
  # Count warnings and errors
  w=$(grep -E "(warning:|WARN)" "${out_log}" "${err_log}" 2>/dev/null | wc -l || echo 0)
  w=$(echo $w | tr -d ' ')
  e=$(grep -E "(error:|ERROR|Failed)" "${err_log}" 2>/dev/null | grep -v "warning:" | wc -l || echo 0)
  e=$(echo $e | tr -d ' ')
  
  # Capitalize first letter of component name
  component_display="$(tr '[:lower:]' '[:upper:]' <<< ${component:0:1})${component:1}"
  
  if [[ $e -gt 0 ]]; then
    echo -e "  ${component_display}: ❌ Failed (${e} errors, ${w} warnings)"
    echo -e "  Errors:"
    grep -E "(error:|ERROR|Failed)" "${out_log}" "${err_log}" 2>/dev/null | grep -v "warning:" | sed 's/^/    /' || true
    echo
  else
    if [[ $w -gt 0 ]]; then
      echo -e "  ${component_display}: ✅ Success (${w} warnings)"
    else
      echo -e "  ${component_display}: ✅ Success (no warnings)"
    fi
  fi
  
  # Check if files were generated
  if [ ! -f "${ROOT_DIR}/components/${component}/api/swagger.json" ]; then
    echo -e "  ⚠️  Warning: No swagger.json file was generated"
  fi
done

print_header "Syncing Postman Collection"

sync_out="${LOG_DIR}/sync.out"
sync_err="${LOG_DIR}/sync.err"

printf "  %-30s " "Updating Postman collection"

# Run the sync-postman script
if "${ROOT_DIR}/scripts/sync-postman.sh" > "${sync_out}" 2> "${sync_err}"; then
  echo -e "✅ SUCCESS"
else
  echo -e "❌ FAILED"
  echo -e "\n  Error details:"
  cat "${sync_err}" | sed 's/^/    /'
  exit 1
fi

echo -e "\n✅ All tasks completed successfully!"

# Clean up temporary logs and artifacts
echo "Cleaning up temporary files..."
rm -rf "${LOG_DIR}"

exit 0
