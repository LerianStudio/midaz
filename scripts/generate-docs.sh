#!/bin/bash
set -euo pipefail

# Root directory of the repo
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Components to process
COMPONENTS=("onboarding" "transaction")

# Temporary log dir
LOG_DIR="${ROOT_DIR}/tmp"
mkdir -p "${LOG_DIR}"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

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

# Check and install swag once at the beginning
ensure_swag_installed() {
  if ! command -v swag >/dev/null 2>&1; then
    echo -e "${YELLOW}Installing swag tool globally...${NC}"
    go install github.com/swaggo/swag/cmd/swag@latest
    echo -e "${GREEN}✓ swag installed successfully${NC}"
  fi
}

# Function to generate docs for a component (runs in parallel)
generate_component_docs() {
  local component=$1
  local component_dir="${ROOT_DIR}/components/${component}"
  local out_log="${LOG_DIR}/${component}.out"
  local err_log="${LOG_DIR}/${component}.err"
  local status_file="${LOG_DIR}/${component}.status"
  
  {
    # Start time tracking
    local start_time=$(date +%s.%N)
    
    # Change to component directory
    cd "${component_dir}" || { 
      echo "FAILED: Could not access directory ${component_dir}" > "${status_file}"
      echo "Failed to change to directory: ${component_dir}" > "${err_log}"
      exit 1
    }
    
    # Generate Swagger documentation using swag
    if swag init -g cmd/app/main.go -o api --parseDependency --parseInternal > "${out_log}" 2> "${err_log}"; then
      echo "SWAG_SUCCESS" >> "${status_file}"
    else
      echo "FAILED: swag init failed" >> "${status_file}"
      exit 1
    fi
    
    # Convert swagger.json to OpenAPI YAML using native Node.js tool
    # This replaces the Docker-based conversion
    if [ -f "${ROOT_DIR}/scripts/swagger-to-openapi.js" ]; then
      if node "${ROOT_DIR}/scripts/swagger-to-openapi.js" \
        "${component_dir}/api/swagger.json" \
        "${component_dir}/api/openapi.yaml" >> "${out_log}" 2>> "${err_log}"; then
        echo "CONVERT_SUCCESS" >> "${status_file}"
      else
        echo "FAILED: OpenAPI conversion failed" >> "${status_file}"
        exit 1
      fi
    else
      # Fallback to Docker if native script doesn't exist
      docker run --rm -v "${component_dir}:/work" --user $(id -u):$(id -g) \
        openapitools/openapi-generator-cli:v5.1.1 generate \
        -i /work/api/swagger.json \
        -g openapi-yaml \
        -o /work/api >> "${out_log}" 2>> "${err_log}"
      
      if [ -f "${component_dir}/api/openapi/openapi.yaml" ]; then
        mv "${component_dir}/api/openapi/openapi.yaml" "${component_dir}/api/openapi.yaml"
        rm -rf "${component_dir}/api/README.md" "${component_dir}/api/.openapi-generator"* "${component_dir}/api/openapi"
        echo "CONVERT_SUCCESS" >> "${status_file}"
      else
        echo "FAILED: Docker conversion failed" >> "${status_file}"
        exit 1
      fi
    fi
    
    # Calculate elapsed time
    local end_time=$(date +%s.%N)
    local elapsed=$(echo "$end_time - $start_time" | bc | tr -d '\n')
    echo "TIME: ${elapsed}" >> "${status_file}"
    
    # Mark as complete
    echo "COMPLETE" >> "${status_file}"
  } &
}

# Main execution
print_header "Generating Swagger API Documentation"

# Ensure swag is installed once
ensure_swag_installed

# Ensure Node.js dependencies are installed once
if [ -f "${ROOT_DIR}/scripts/package.json" ]; then
  echo -e "${YELLOW}Checking npm dependencies...${NC}"
  (cd "${ROOT_DIR}/scripts" && npm install --silent)
fi

# Start parallel component processing
echo -e "Processing components in parallel...\n"

# Launch all components in parallel
for component in "${COMPONENTS[@]}"; do
  generate_component_docs "${component}"
  
  # Capitalize first letter for display
  component_display="$(tr '[:lower:]' '[:upper:]' <<< ${component:0:1})${component:1}"
  echo -e "  ${component_display}: ${YELLOW}Processing...${NC}"
done

# Wait for all background jobs to complete
wait

# Display results
echo -e "\nChecking results..."

for component in "${COMPONENTS[@]}"; do
  component_display="$(tr '[:lower:]' '[:upper:]' <<< ${component:0:1})${component:1}"
  status_file="${LOG_DIR}/${component}.status"
  
  if [ -f "${status_file}" ] && grep -q "COMPLETE" "${status_file}"; then
    # Extract time (get the last TIME: entry)
    time_taken=$(grep "TIME:" "${status_file}" | tail -1 | cut -d' ' -f2)
    printf "  %-20s ${GREEN}✅ SUCCESS${NC} (%.2fs)\n" "${component_display}" "${time_taken}"
  else
    printf "  %-20s ${RED}❌ FAILED${NC}\n" "${component_display}"
  fi
done

# Summary of Swagger documentation generation
print_header "Documentation Generation Summary"

all_success=true
for component in "${COMPONENTS[@]}"; do
  out_log="${LOG_DIR}/${component}.out"
  err_log="${LOG_DIR}/${component}.err"
  status_file="${LOG_DIR}/${component}.status"
  
  # Count warnings and errors
  w=$(grep -E "(warning:|WARN)" "${out_log}" "${err_log}" 2>/dev/null | wc -l || echo 0)
  w=$(echo $w | tr -d ' ')
  e=$(grep -E "(error:|ERROR|Failed)" "${err_log}" 2>/dev/null | grep -v "warning:" | wc -l || echo 0)
  e=$(echo $e | tr -d ' ')
  
  # Capitalize first letter of component name
  component_display="$(tr '[:lower:]' '[:upper:]' <<< ${component:0:1})${component:1}"
  
  if [[ $e -gt 0 ]] || ! grep -q "COMPLETE" "${status_file}" 2>/dev/null; then
    echo -e "  ${component_display}: ${RED}❌ Failed${NC} (${e} errors, ${w} warnings)"
    if [[ $e -gt 0 ]]; then
      echo -e "  Errors:"
      grep -E "(error:|ERROR|Failed)" "${out_log}" "${err_log}" 2>/dev/null | grep -v "warning:" | head -5 | sed 's/^/    /' || true
    fi
    all_success=false
  else
    if [[ $w -gt 0 ]]; then
      echo -e "  ${component_display}: ${GREEN}✅ Success${NC} (${w} warnings)"
    else
      echo -e "  ${component_display}: ${GREEN}✅ Success${NC} (no warnings)"
    fi
  fi
  
  # Check if files were generated
  if [ ! -f "${ROOT_DIR}/components/${component}/api/swagger.json" ]; then
    echo -e "  ${YELLOW}⚠️  Warning: No swagger.json file was generated${NC}"
    all_success=false
  fi
done

# Exit early if documentation generation failed
if [ "$all_success" = false ]; then
  echo -e "\n${RED}❌ Documentation generation failed. Skipping Postman sync.${NC}"
  rm -rf "${LOG_DIR}"
  exit 1
fi

print_header "Syncing Postman Collection"

sync_out="${LOG_DIR}/sync.out"
sync_err="${LOG_DIR}/sync.err"

printf "  %-30s " "Updating Postman collection"

# Run the sync-postman script
if "${ROOT_DIR}/scripts/sync-postman.sh" > "${sync_out}" 2> "${sync_err}"; then
  echo -e "${GREEN}✅ SUCCESS${NC}"
else
  echo -e "${RED}❌ FAILED${NC}"
  echo -e "\n  Error details:"
  head -10 "${sync_err}" | sed 's/^/    /'
  exit 1
fi

echo -e "\n${GREEN}✅ All tasks completed successfully!${NC}"

# Clean up temporary logs and artifacts
echo "Cleaning up temporary files..."
rm -rf "${LOG_DIR}"

exit 0