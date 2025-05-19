#!/usr/bin/env bash
# ============================================================================
# Midaz Install Script Testing Framework
# ============================================================================
#
# This script tests the Midaz install.sh script across multiple Linux
# distributions and architectures using Docker containers.
#
# It builds and runs Docker containers for each target environment,
# executes the install script, and generates a report of the results.
#
# Usage:
#   ./run-tests.sh [OPTIONS]
#
# Options:
#   --help          Display this help message
#   --clean         Remove all test artifacts before running
#   --report-only   Generate report from existing logs without running tests
#   --parallel      Run tests in parallel (default: sequential)
#   --distro NAME   Test only the specified distribution

set -euo pipefail

# Directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="${SCRIPT_DIR}/logs"
REPORT_FILE="${SCRIPT_DIR}/test-report.md"

# No color codes - removed for better compatibility

# Default options
CLEAN=1
REPORT_ONLY=0
PARALLEL=0
SPECIFIC_DISTRO=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --help)
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --help          Display this help message"
      echo "  --clean         Remove all test artifacts before running"
      echo "  --report-only   Generate report from existing logs without running tests"
      echo "  --parallel      Run tests in parallel (default: sequential)"
      echo "  --distro NAME   Test only the specified distribution"
      exit 0
      ;;
    --clean)
      CLEAN=1
      shift
      ;;
    --report-only)
      REPORT_ONLY=1
      shift
      ;;
    --parallel)
      PARALLEL=1
      shift
      ;;
    --distro)
      SPECIFIC_DISTRO="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Function to print messages
log() {
  local msg=$1
  echo "${msg}"
}

# Clean up previous test artifacts if requested
if [[ $CLEAN -eq 1 ]]; then
  log "Cleaning up previous test artifacts..."
  rm -rf "${LOG_DIR}"
  docker-compose -f "${SCRIPT_DIR}/docker-compose.yml" down -v --remove-orphans
  docker system prune -f
fi

# Create logs directory
mkdir -p "${LOG_DIR}"

# Run the tests unless report-only is specified
if [[ $REPORT_ONLY -eq 0 ]]; then
  log "Starting install script tests across distributions..."
  
  # If a specific distro is specified, only test that one
  if [[ -n "${SPECIFIC_DISTRO}" ]]; then
    log "Testing only ${SPECIFIC_DISTRO} distribution"
    docker-compose -f "${SCRIPT_DIR}/docker-compose.yml" up --build "${SPECIFIC_DISTRO}"
  else
    # Run tests in parallel or sequentially
    if [[ $PARALLEL -eq 1 ]]; then
      log "Running tests in parallel mode"
      docker-compose -f "${SCRIPT_DIR}/docker-compose.yml" up --build --no-recreate
    else
      log "Running tests sequentially"
      
      # Get all service names from docker-compose.yml
      SERVICES=$(docker-compose -f "${SCRIPT_DIR}/docker-compose.yml" config --services)
      
      for service in $SERVICES; do
        log "Testing on ${service}..."
        docker-compose -f "${SCRIPT_DIR}/docker-compose.yml" up --build --no-recreate "${service}"
      done
    fi
  fi
fi

# Generate the test report
generate_report() {
  log "Generating test report..."
  
  # Create report header
  cat > "${REPORT_FILE}" << EOF
# Midaz Install Script Test Report

Generated: $(date)

## Summary

| Distribution | Status | Log |
|-------------|--------|-----|
EOF
  
  # Add results for each distribution
  local total=0
  local passed=0
  
  for status_file in "${LOG_DIR}"/*.status; do
    if [[ -f "${status_file}" ]]; then
      local distro=$(basename "${status_file}" .status)
      local status=$(cat "${status_file}")
      local status_icon="❌"
      
      ((total++))
      
      if [[ "${status}" == "SUCCESS" ]]; then
        status_icon="✅"
        ((passed++))
      fi
      
      echo "| ${distro} | ${status_icon} ${status} | [View Log](./logs/${distro}.log) |" >> "${REPORT_FILE}"
    fi
  done
  
  # Add summary statistics
  if [[ $total -gt 0 ]]; then
    local pass_rate=$((passed * 100 / total))
    
    cat >> "${REPORT_FILE}" << EOF

## Statistics

- **Total Tests:** ${total}
- **Passed:** ${passed}
- **Failed:** $((total - passed))
- **Pass Rate:** ${pass_rate}%

EOF
    
    # Add detailed failure information if there are failures
    if [[ $passed -lt $total ]]; then
      cat >> "${REPORT_FILE}" << EOF
## Failure Details

EOF
      
      for status_file in "${LOG_DIR}"/*.status; do
        if [[ -f "${status_file}" ]]; then
          local distro=$(basename "${status_file}" .status)
          local status=$(cat "${status_file}")
          
          if [[ "${status}" != "SUCCESS" ]]; then
            local log_file="${LOG_DIR}/${distro}.log"
            
            cat >> "${REPORT_FILE}" << EOF
### ${distro} Failure

\`\`\`
$(grep -i "error\|fail\|exception" "${log_file}" | tail -n 20)
\`\`\`

EOF
          fi
        fi
      done
    fi
  else
    echo "No test results found." >> "${REPORT_FILE}"
  fi
  
  log "Test report generated at ${REPORT_FILE}"
}

generate_report

# Print summary to console
if [[ -f "${REPORT_FILE}" ]]; then
  total=$(grep -c "^|" "${REPORT_FILE}" | awk '{print $1-2}')
  passed=$(grep -c "✅" "${REPORT_FILE}")
  
  if [[ $total -gt 0 ]]; then
    pass_rate=$((passed * 100 / total))
    
    echo ""
    log "Test Summary:"
    log "Total Tests: ${total}"
    log "Passed: ${passed}"
    log "Failed: $((total - passed))"
    log "Pass Rate: ${pass_rate}%"
    
    if [[ $passed -eq $total ]]; then
      log "✅ All tests passed!"
    else
      log "⚠️ Some tests failed. See ${REPORT_FILE} for details."
    fi
  else
    log "No test results found."
  fi
fi

exit 0
