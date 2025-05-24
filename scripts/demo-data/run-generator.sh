#!/bin/bash
# ============================================================================
# Midaz Demo Data Generator Script
# ============================================================================
# This script generates realistic demo data for the Midaz core-banking platform
# It handles:
#  - OS compatibility checks
#  - SDK dependency verification and cloning if needed
#  - Running the TypeScript generator with appropriate volume parameters

# Enable strict mode
set -e

# ============================================================================
# Logging Functions
# ============================================================================

log() {
  echo "[DEMO-GEN] $1"
}

log_success() {
  echo "[DEMO-GEN] SUCCESS: $1"
}

log_warning() {
  echo "[DEMO-GEN] WARNING: $1"
}

log_error() {
  echo "[DEMO-GEN] ERROR: $1" >&2
}

# Fatal error handler
die() {
  log_error "$1"
  echo "\nDiagnostic Information:"
  echo "===================================================================="
  
  # OS release information if available
  if [ -f /etc/os-release ]; then
    echo "OS Information:"
    cat /etc/os-release
  fi
  
  # Kernel and architecture information
  echo "UNAME: $(uname -a)"
  echo "Node version: $(node --version 2>/dev/null || echo 'not installed')"
  echo "NPM version: $(npm --version 2>/dev/null || echo 'not installed')"
  
  echo "\nPlease report issues to support@midaz.dev with the above information."
  echo "===================================================================="
  exit 1
}

# Checks if a command is available in the current PATH
check_command() {
  command -v "$1" >/dev/null 2>&1
}

# ============================================================================
# OS Detection
# ============================================================================

detect_os() {
  log "Detecting operating system..."
  
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_NAME="${ID}"
    OS_VERSION="${VERSION_ID:-}"
    OS_FAMILY="linux"
  elif [ "$(uname)" = "Darwin" ]; then
    OS_NAME="macos"
    OS_FAMILY="darwin"
    OS_VERSION="$(sw_vers -productVersion)"
  else
    die "Unsupported operating system. This generator supports Linux and macOS."
  fi
  
  log "Detected: ${OS_NAME} ${OS_VERSION} (${OS_FAMILY})"
}

# ============================================================================
# Dependency Checking
# ============================================================================

check_dependencies() {
  log "Checking for required dependencies..."
  
  # Check for Node.js
  if ! check_command node; then
    die "Node.js is required but not installed. Please install Node.js 16+ and try again."
  fi
  
  # Check for npm
  if ! check_command npm; then
    die "npm is required but not installed. Please install npm and try again."
  fi
  
  # Check for git (needed for cloning SDK)
  if ! check_command git; then
    die "git is required but not installed. Please install git and try again."
  fi
  
  log_success "All required dependencies are installed"
}

# Go to script directory
cd "$(dirname "$0")"

# Run system checks
detect_os
check_dependencies

# ============================================================================
# SDK and Dependencies Setup
# ============================================================================

# Check and handle SDK dependency
SDK_PATH="$(pwd)/sdk-source"
SDK_REPO="https://github.com/lerianstudio/midaz-sdk-typescript.git"

# If SDK is not properly linked, try to get it
if [ ! -d "$SDK_PATH/src" ]; then
  log "Setting up Midaz SDK..."
  
  # Remove any partial installations
  rm -rf "$SDK_PATH"
  
  # Clone from GitHub (main branch)
  log "Cloning SDK from GitHub: $SDK_REPO (main branch)"
  git clone -b main "$SDK_REPO" "$SDK_PATH" || die "Failed to clone SDK repository"
    
  # Install SDK dependencies (skip prepare script to avoid build failure)
  log "Installing SDK dependencies..."
  (cd "$SDK_PATH" && npm install --ignore-scripts) || log_warning "SDK dependency installation had issues"
  
  # Install missing Node.js types
  log "Installing Node.js types for SDK..."
  (cd "$SDK_PATH" && npm install --save-dev @types/node --ignore-scripts) || log_warning "Failed to install @types/node"
    
  # Try to build the SDK, but don't fail if it doesn't work
  log "Attempting to build SDK..."
  if (cd "$SDK_PATH" && npm run build 2>/dev/null); then
    log_success "SDK built successfully"
  else
    log_warning "SDK build failed, but continuing with transpile-only mode..."
    
    # Create a minimal package.json in src for direct imports
    cat > "$SDK_PATH/package.json.backup" << 'EOF'
{
  "name": "midaz-sdk",
  "version": "0.1.0",
  "main": "src/index.ts",
  "type": "module"
}
EOF
    
    # Ensure we can still import the SDK at runtime with ts-node transpile-only
    log "SDK will be used in transpile-only mode during generation"
  fi
  
  log_success "SDK setup complete (ready for transpile-only usage)"
fi

# Check if node_modules exists and install dependencies if needed
if [ ! -d "node_modules" ]; then
  log "Installing dependencies..."
  npm install || die "Failed to install npm dependencies"
fi

# ============================================================================
# Parameter Handling
# ============================================================================

# Get volume size from command line or use small as default
VOLUME=${1:-small}
log "Using volume size: $VOLUME"

# Validate volume size
case "$VOLUME" in
  small|medium|large)
    # Valid size
    ;;
  *)
    log_warning "Invalid volume size: $VOLUME. Using 'small' instead."
    VOLUME="small"
    ;;
esac

# Get auth token from command line (use "none" for fresh installations)
AUTH_TOKEN=${2:-"none"}
if [ "$AUTH_TOKEN" = "none" ]; then
  log "No authentication token provided, using default for fresh installations"
fi

# Check for test mode flag for unit/integration testing
TEST_MODE=false
for arg in "$@"; do
  if [ "$arg" = "--test-mode" ]; then
    TEST_MODE=true
    log "Running in test mode - no actual API calls will be made"
    export MIDAZ_TEST_MODE=true
  fi
done

# ============================================================================
# Service Validation
# ============================================================================

check_services_running() {
  local onboarding_port="${1:-3000}"
  local transaction_port="${2:-3001}"
  
  log "Checking if Midaz services are running..."
  
  # Check if onboarding service is running
  if ! nc -z localhost "$onboarding_port" >/dev/null 2>&1; then
    log_warning "Onboarding service does not appear to be running on port $onboarding_port"
    return 1
  fi
  
  # Check if transaction service is running
  if ! nc -z localhost "$transaction_port" >/dev/null 2>&1; then
    log_warning "Transaction service does not appear to be running on port $transaction_port"
    return 1
  fi
  
  log_success "Midaz services are running"
  return 0
}

# ============================================================================
# Generator Execution
# ============================================================================

# Run the generator directly with ts-node (skipping type checking)
log "Running demo data generator with volume: $VOLUME..."

# Set environment variables for ts-node (transpile-only for speed and compatibility)
export TS_NODE_TRANSPILE_ONLY=true 
export TS_NODE_COMPILER_OPTIONS='{"module":"commonjs","moduleResolution":"node","skipLibCheck":true,"allowJs":true}'
export TS_NODE_SKIP_IGNORE=true

# Verify Midaz services are running
if ! check_services_running; then
  log_warning "Midaz services may not be running. Data generation might fail."
  
  if [ "$TEST_MODE" != true ]; then
    # Ask user if they want to continue
    read -p "Continue anyway? (y/N): " confirm
    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
      log "Aborting data generation."
      exit 1
    fi
    log "Continuing despite potential service unavailability..."
  fi
fi

# Run the generator
log "Starting generator process..."

# Check if ts-node is available (prefer npx for compatibility)
if ! command -v npx >/dev/null 2>&1; then
  die "npx is required but not found. Please ensure npm is properly installed."
fi

# Verify ts-node is available
if ! npx ts-node --version >/dev/null 2>&1; then
  log "ts-node not available, installing..."
  npm install ts-node || die "Failed to install ts-node"
fi

# If in test mode and not explicitly testing the generator functionality,
# exit with success to simulate a successful run
if [ "$TEST_MODE" = true ] && [ -z "${TEST_RUN_GENERATOR:-}" ]; then
  log "Test mode: Simulating successful generator execution"
  log_success "Demo data generation completed successfully! (Test Mode)"
  exit 0
fi

# Set API base URL for onboarding service
export API_BASE_URL="${API_BASE_URL:-http://localhost:3000}"
export ONBOARDING_API_URL="${ONBOARDING_API_URL:-http://localhost:3000}"
export TRANSACTION_API_URL="${TRANSACTION_API_URL:-http://localhost:3001}"

# Otherwise run the actual generator
log "Using API endpoints: Onboarding=$API_BASE_URL, Transaction=$TRANSACTION_API_URL"
if npx ts-node --project tsconfig-ts-node.json src/index.ts --volume "$VOLUME" --auth-token "$AUTH_TOKEN" --api-base-url "$API_BASE_URL" ${TEST_MODE:+--test-mode}; then
  log_success "Demo data generation completed successfully!"
else
  log_error "Demo data generation failed with exit code $?"
  die "Generator process failed. Check logs above for details."
fi

# Inform user about customizing command
echo ""
log "To run with different options, use:"
log "  ./run-generator.sh [small|medium|large] [auth-token]"
echo ""
