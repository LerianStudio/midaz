#!/bin/bash
# Documentation Tools Installation Script
# Safely installs required tools for documentation generation with race condition protection

set -euo pipefail

# Configuration
SWAG_VERSION="v1.16.3"
LOCK_DIR="/tmp/midaz-docs-tools"
SWAG_LOCK_FILE="${LOCK_DIR}/swag.lock"
TIMEOUT_SECONDS=300

# Detect platform for cross-platform compatibility
PLATFORM=$(uname -s)
case $PLATFORM in
    "Darwin")
        IS_MACOS=true
        ;;
    "Linux")
        IS_LINUX=true
        ;;
    *)
        IS_OTHER=true
        ;;
esac

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] $1"
}

log_success() {
    echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] ${GREEN}$1${NC}"
}

log_warning() {
    echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] ${YELLOW}$1${NC}"
}

log_error() {
    echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] ${RED}$1${NC}" >&2
}

# Create lock directory
create_lock_dir() {
    mkdir -p "${LOCK_DIR}" 2>/dev/null || true
}

# Cleanup function
cleanup() {
    if [ -n "${SWAG_LOCK_PID:-}" ]; then
        # Remove lock if we own it
        if [ -f "${SWAG_LOCK_FILE}" ] && [ "$(cat "${SWAG_LOCK_FILE}" 2>/dev/null)" = "${SWAG_LOCK_PID}" ]; then
            rm -f "${SWAG_LOCK_FILE}"
        fi
    fi
}

# Set up cleanup trap
trap cleanup EXIT INT TERM

# Cross-platform timeout implementation
run_with_timeout() {
    local timeout_duration="$1"
    shift
    local command_to_run="$@"
    
    # Try native timeout first (Linux)
    if command -v timeout >/dev/null 2>&1; then
        timeout "${timeout_duration}" $command_to_run
        return $?
    fi
    
    # Try GNU timeout (if installed via Homebrew on macOS)
    if command -v gtimeout >/dev/null 2>&1; then
        gtimeout "${timeout_duration}" $command_to_run
        return $?
    fi
    
    # Fallback: implement timeout using background process and kill
    # This works on any UNIX-like system including macOS
    local temp_result_file="/tmp/midaz_timeout_result_$$"
    local temp_pid_file="/tmp/midaz_timeout_pid_$$"
    
    # Run command in background and capture its PID
    (
        $command_to_run
        echo $? > "${temp_result_file}"
    ) &
    local cmd_pid=$!
    echo $cmd_pid > "${temp_pid_file}"
    
    # Start timeout monitoring in background
    (
        sleep "${timeout_duration}"
        if kill -0 $cmd_pid 2>/dev/null; then
            log_warning "Command timed out after ${timeout_duration} seconds, terminating..."
            kill -TERM $cmd_pid 2>/dev/null || true
            sleep 2
            if kill -0 $cmd_pid 2>/dev/null; then
                kill -KILL $cmd_pid 2>/dev/null || true
            fi
            echo 124 > "${temp_result_file}" # timeout exit code
        fi
    ) &
    local timeout_pid=$!
    
    # Wait for command to complete
    wait $cmd_pid 2>/dev/null
    local cmd_exit_code=$?
    
    # Kill timeout monitor
    kill $timeout_pid 2>/dev/null || true
    wait $timeout_pid 2>/dev/null || true
    
    # Clean up temp files
    local result_code=$cmd_exit_code
    if [ -f "${temp_result_file}" ]; then
        result_code=$(cat "${temp_result_file}" 2>/dev/null || echo $cmd_exit_code)
        rm -f "${temp_result_file}"
    fi
    rm -f "${temp_pid_file}"
    
    return $result_code
}

# Function to acquire lock with timeout
acquire_lock() {
    local lock_file="$1"
    local timeout="$2"
    local start_time=$(date +%s)
    
    while true; do
        # Try to acquire lock
        if (set -C; echo $$ > "${lock_file}") 2>/dev/null; then
            return 0
        fi
        
        # Check timeout
        local current_time=$(date +%s)
        if [ $((current_time - start_time)) -ge ${timeout} ]; then
            return 1
        fi
        
        # Check if the process holding the lock is still alive
        if [ -f "${lock_file}" ]; then
            local lock_pid=$(cat "${lock_file}" 2>/dev/null || echo "")
            if [ -n "${lock_pid}" ] && ! kill -0 "${lock_pid}" 2>/dev/null; then
                log_warning "Removing stale lock file (PID ${lock_pid} not found)"
                rm -f "${lock_file}"
                continue
            fi
        fi
        
        # Wait a bit before retrying
        sleep 1
    done
}

# Function to install swag safely
install_swag() {
    log "Checking swag installation..."
    
    # Check if swag is already available
    if command -v swag >/dev/null 2>&1; then
        local current_version=$(swag --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
        log_success "swag is already installed (version: ${current_version})"
        return 0
    fi
    
    log "swag not found, attempting to install..."
    
    # Acquire lock for swag installation
    if acquire_lock "${SWAG_LOCK_FILE}" "${TIMEOUT_SECONDS}"; then
        SWAG_LOCK_PID=$$
        log "Lock acquired for swag installation (PID: $$)"
        
        # Double-check if swag was installed while waiting for lock
        if command -v swag >/dev/null 2>&1; then
            log_success "swag was installed by another process"
            return 0
        fi
        
        # Install swag with version pinning
        log "Installing swag ${SWAG_VERSION}..."
        if go install "github.com/swaggo/swag/cmd/swag@${SWAG_VERSION}"; then
            log_success "swag ${SWAG_VERSION} installed successfully"
            
            # Verify installation
            if command -v swag >/dev/null 2>&1; then
                local installed_version=$(swag --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
                log_success "Verification successful - swag version: ${installed_version}"
            else
                log_error "Installation verification failed - swag command not found in PATH"
                return 1
            fi
        else
            log_error "Failed to install swag"
            return 1
        fi
    else
        log_error "Failed to acquire lock for swag installation within ${TIMEOUT_SECONDS} seconds"
        return 1
    fi
}

# Function to verify Docker is available
verify_docker() {
    log "Verifying Docker availability..."
    
    if ! command -v docker >/dev/null 2>&1; then
        log_error "Docker is not installed or not in PATH"
        return 1
    fi
    
    if ! docker info >/dev/null 2>&1; then
        log_error "Docker daemon is not running"
        return 1
    fi
    
    log_success "Docker is available and running"
    return 0
}

# Function to verify Go is available
verify_go() {
    log "Verifying Go installation..."
    
    if ! command -v go >/dev/null 2>&1; then
        log_error "Go is not installed or not in PATH"
        return 1
    fi
    
    local go_version=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1 || echo "unknown")
    log_success "Go is available (version: ${go_version})"
    return 0
}

# Main installation function
install_docs_tools() {
    log "Starting documentation tools installation..."
    
    create_lock_dir
    
    # Verify prerequisites
    if ! verify_go; then
        log_error "Go is required but not available"
        return 1
    fi
    
    if ! verify_docker; then
        log_error "Docker is required but not available"
        return 1
    fi
    
    # Install tools
    if ! install_swag; then
        log_error "Failed to install swag"
        return 1
    fi
    
    log_success "All documentation tools installed successfully"
    return 0
}

# Main execution
main() {
    case "${1:-install}" in
        "install")
            install_docs_tools
            ;;
        "verify")
            verify_go && verify_docker && command -v swag >/dev/null 2>&1
            ;;
        "clean")
            log "Cleaning up lock files..."
            rm -rf "${LOCK_DIR}"
            log_success "Lock files cleaned"
            ;;
        *)
            echo "Usage: $0 [install|verify|clean]"
            echo "  install  - Install documentation tools (default)"
            echo "  verify   - Verify tools are available"
            echo "  clean    - Clean up lock files"
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@"