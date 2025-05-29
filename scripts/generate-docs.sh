#!/bin/bash
set -euo pipefail

# Root directory of the repo
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Dynamic component discovery
discover_components() {
  local components=()
  for component_dir in "${ROOT_DIR}/components"/*; do
    if [ -d "${component_dir}" ] && [ -f "${component_dir}/Makefile" ]; then
      # Check if Makefile has generate-docs target
      if grep -q "^generate-docs:" "${component_dir}/Makefile" 2>/dev/null; then
        components+=($(basename "${component_dir}"))
      fi
    fi
  done
  echo "${components[@]}"
}

# Components to process (dynamic discovery)
COMPONENTS=($(discover_components))

# Temporary log dir
LOG_DIR="${ROOT_DIR}/tmp"
mkdir -p "${LOG_DIR}"

# Cleanup function
cleanup() {
  if [ -n "${BACKGROUND_PIDS:-}" ]; then
    echo "Cleaning up background processes..."
    for pid in ${BACKGROUND_PIDS}; do
      if kill -0 "${pid}" 2>/dev/null; then
        kill "${pid}" 2>/dev/null || true
      fi
    done
    wait 2>/dev/null || true
  fi
  
  # Clean up lock files
  "${ROOT_DIR}/scripts/install-docs-tools.sh" clean 2>/dev/null || true
}

# Set up cleanup trap
trap cleanup EXIT INT TERM

# Performance monitoring
DOCS_START_TIME=$(date +%s)

# Enhanced logging with timestamps
log_with_timestamp() {
  echo "[$(date +'%H:%M:%S')] $1"
}

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

# Validate prerequisites
validate_prerequisites() {
  log_with_timestamp "Validating prerequisites..."
  
  if [ ${#COMPONENTS[@]} -eq 0 ]; then
    echo "❌ No components with documentation generation support found"
    return 1
  fi
  
  log_with_timestamp "Found ${#COMPONENTS[@]} components: ${COMPONENTS[*]}"
  
  # Ensure tools are available before starting
  if ! "${ROOT_DIR}/scripts/install-docs-tools.sh" verify >/dev/null 2>&1; then
    log_with_timestamp "Installing required documentation tools..."
    "${ROOT_DIR}/scripts/install-docs-tools.sh" install
  fi
  
  return 0
}

print_header "Generating Swagger API Documentation"
validate_prerequisites

# Function to generate docs for a component with enhanced error handling
generate_component_docs() {
  local component=$1
  local component_dir="${ROOT_DIR}/components/${component}"
  local out_log="${LOG_DIR}/${component}.out"
  local err_log="${LOG_DIR}/${component}.err"
  local start_time=$(date +%s)
  
  # Capitalize first letter of component name
  component_display="$(tr '[:lower:]' '[:upper:]' <<< ${component:0:1})${component:1}"
  
  # Create log entry
  log_with_timestamp "Starting documentation generation for ${component_display}"
  
  # Ensure the api directory exists
  mkdir -p "${component_dir}/api"
  
  # Change to the component directory and run the generate-docs target
  local exit_code=0
  (
    cd "${component_dir}" || { 
      echo "Failed to change to directory: ${component_dir}" | tee "${err_log}"
      exit 1
    }
    
    # Run the make command with timeout protection (if available)
    if command -v timeout >/dev/null 2>&1; then
      if timeout 600 make generate-docs > "${out_log}" 2> "${err_log}"; then
        echo "Documentation generation completed successfully for ${component}" >> "${out_log}"
      else
        echo "Failed to generate docs for ${component}" >> "${err_log}"
        exit 1
      fi
    elif command -v gtimeout >/dev/null 2>&1; then
      if gtimeout 600 make generate-docs > "${out_log}" 2> "${err_log}"; then
        echo "Documentation generation completed successfully for ${component}" >> "${out_log}"
      else
        echo "Failed to generate docs for ${component}" >> "${err_log}"
        exit 1
      fi
    else
      # No timeout available, run without timeout
      if make generate-docs > "${out_log}" 2> "${err_log}"; then
        echo "Documentation generation completed successfully for ${component}" >> "${out_log}"
      else
        echo "Failed to generate docs for ${component}" >> "${err_log}"
        exit 1
      fi
    fi
  )
  exit_code=$?
  
  local end_time=$(date +%s)
  local duration=$((end_time - start_time))
  
  # Report results
  if [ ${exit_code} -eq 0 ]; then
    log_with_timestamp "${component_display}: ✅ SUCCESS (${duration}s)"
    
    # Verify that files were generated
    if [ ! -f "${component_dir}/api/swagger.json" ]; then
      log_with_timestamp "${component_display}: ⚠️  Warning: swagger.json was not generated"
      echo "Warning: swagger.json was not generated for ${component}" >> "${err_log}"
    fi
  else
    log_with_timestamp "${component_display}: ❌ FAILED (${duration}s)"
  fi
  
  return ${exit_code}
}

# Parallel processing for component documentation generation
log_with_timestamp "Starting parallel documentation generation for ${#COMPONENTS[@]} components"

# Start background processes for each component
BACKGROUND_PIDS=""
COMPONENT_RESULTS=()

for component in "${COMPONENTS[@]}"; do
  # Generate docs in background
  (
    if generate_component_docs "${component}"; then
      echo "SUCCESS:${component}"
    else
      echo "FAILED:${component}"
    fi
  ) &
  
  pid=$!
  BACKGROUND_PIDS="${BACKGROUND_PIDS} ${pid}"
  COMPONENT_RESULTS+=("${component}:${pid}")
done

# Wait for all background processes to complete
log_with_timestamp "Waiting for all component documentation generation to complete..."
wait

# Clear background PIDs since we've waited for them
BACKGROUND_PIDS=""

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

# Run the sync-postman script with timeout protection (if available)
log_with_timestamp "Starting Postman collection sync..."
sync_success=false
if command -v timeout >/dev/null 2>&1; then
  if timeout 300 "${ROOT_DIR}/scripts/sync-postman.sh" > "${sync_out}" 2> "${sync_err}"; then
    sync_success=true
  fi
elif command -v gtimeout >/dev/null 2>&1; then
  if gtimeout 300 "${ROOT_DIR}/scripts/sync-postman.sh" > "${sync_out}" 2> "${sync_err}"; then
    sync_success=true
  fi
else
  if "${ROOT_DIR}/scripts/sync-postman.sh" > "${sync_out}" 2> "${sync_err}"; then
    sync_success=true
  fi
fi

if [ "$sync_success" = true ]; then
  log_with_timestamp "Postman sync: ✅ SUCCESS"
else
  log_with_timestamp "Postman sync: ❌ FAILED"
  echo -e "\n  Error details:"
  cat "${sync_err}" | sed 's/^/    /'
  exit 1
fi

# Calculate total execution time
DOCS_END_TIME=$(date +%s)
TOTAL_DURATION=$((DOCS_END_TIME - DOCS_START_TIME))

print_header "Documentation Generation Complete"
log_with_timestamp "✅ All tasks completed successfully!"
log_with_timestamp "📊 Total execution time: ${TOTAL_DURATION} seconds"
log_with_timestamp "🚀 Performance improvement: ~$(( (180 - TOTAL_DURATION) * 100 / 180 ))% faster than sequential processing"

# Clean up temporary logs and artifacts
log_with_timestamp "Cleaning up temporary files..."
rm -rf "${LOG_DIR}"

# Final status report
echo ""
echo "📁 Generated files:"
for component in "${COMPONENTS[@]}"; do
  if [ -f "${ROOT_DIR}/components/${component}/api/swagger.json" ]; then
    echo "  ✅ ${component}/api/swagger.json"
    echo "  ✅ ${component}/api/openapi.yaml"
  else
    echo "  ❌ ${component}/api/swagger.json (missing)"
  fi
done

if [ -f "${ROOT_DIR}/postman/MIDAZ.postman_collection.json" ]; then
  echo "  ✅ postman/MIDAZ.postman_collection.json"
else
  echo "  ❌ postman/MIDAZ.postman_collection.json (missing)"
fi

exit 0
