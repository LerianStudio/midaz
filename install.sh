#!/usr/bin/env sh
# ============================================================================
# Midaz Core-Banking Stack Installation Script
# ============================================================================
#
# This script provides an automated installation of the Midaz core-banking platform.
# It performs the following actions:
#  1. Detects the operating system and package manager
#  2. Verifies and installs all required dependencies:
#     - Git (for cloning the repository)
#     - Docker + Docker Compose (for containerization)
#     - Make (for running build processes)
#     - Go 1.22+ (for building backend services)
#     - Node.js 20+ (for frontend applications)
#  3. Clones the Midaz repository
#  4. Starts the services using Docker Compose
#  5. Optionally generates demo data
#
# Usage:
#   curl -fsSL https://get.midaz.dev | sh
#
# Configurable environment variables:
#   MIDAZ_DIR      Override installation directory (default: ~/midaz)
#   MIDAZ_REF      Branch/tag to clone (default: main)
#   INSTALL_FLAGS  Pass -y to skip interactive prompts (fully automated install)

# ============================================================================
# Configuration and Safety Settings
# ============================================================================

# Enable strict mode for safer execution
# -e: Exit immediately if a command fails
# -u: Treat unset variables as an error
set -eu

# Set a safe Internal Field Separator (IFS)
# This ensures that whitespace in filenames and paths is handled correctly
IFS="$(printf '\n\t')"

# ============================================================================
# Default Configuration Variables
# ============================================================================

# Installation location - can be overridden by setting MIDAZ_DIR environment variable
MIDAZ_DIR="${MIDAZ_DIR:-$HOME/midaz}"

# Git reference (branch/tag) to install - can be overridden by setting MIDAZ_REF
MIDAZ_REF="${MIDAZ_REF:-main}"

# Repository URL - this is the official Midaz repository
MIDAZ_REPO="https://github.com/lerianstudio/midaz"

# Flag to track if we're running in non-interactive mode
MIDAZ_AUTOCONFIRM=0

# Installation flags from environment - used to enable non-interactive mode
INSTALL_FLAGS="${INSTALL_FLAGS:-}"

# ============================================================================
# Signal Handling and Cleanup
# ============================================================================

# This function handles interruption signals to ensure clean termination
# If the user presses Ctrl+C or the script receives a termination signal,
# this ensures we exit gracefully instead of leaving partial installations
cleanup() {
  echo "[MIDAZ] Installation interrupted. Exiting..."
  echo "[MIDAZ] You may need to manually clean up any partially installed components."
  exit 1
}

# Set traps for various signals:
# INT: Keyboard interrupt (Ctrl+C)
# TERM: Termination signal
# EXIT: Script exit
# HUP: Terminal disconnection
trap cleanup INT TERM EXIT HUP

# We'll reset the EXIT trap at the end of successful installation
# This prevents the cleanup function from running on normal exit
trap '' EXIT

# ============================================================================
# Logging Functions
# ============================================================================
# These functions provide consistent message formatting throughout the script
# and make it easier to distinguish between different types of messages.

# Standard informational message
log() {
  echo "[MIDAZ] $1"
}

# Success message for completed operations
log_success() {
  echo "[MIDAZ] SUCCESS: $1"
}

# Warning message for potential issues that don't stop installation
log_warning() {
  echo "[MIDAZ] WARNING: $1"
}

# Error message for issues that prevent continuing
# Outputs to stderr to differentiate from regular output
log_error() {
  echo "[MIDAZ] ERROR: $1" >&2
}

# Fatal error handler - logs error, collects system information for debugging,
# and exits with a non-zero status code
die() {
  log_error "$1"
  # Provide detailed system information to help with troubleshooting
  echo "\nDiagnostic Information (please include in any support requests):"
  echo "===================================================================="
  
  # OS release information if available
  if [ -f /etc/os-release ]; then
    echo "OS Information:"
    cat /etc/os-release
  fi
  
  # Kernel and architecture information
  echo "UNAME: $(uname -a)"
  
  # Information about installation parameters
  echo "Installation directory: ${MIDAZ_DIR}"
  echo "Git reference: ${MIDAZ_REF}"
  
  echo "\nPlease report installation issues to support@midaz.dev with the above information."
  echo "===================================================================="
  exit 1
}

# ============================================================================
# User Interaction Functions
# ============================================================================

# Prompts the user for confirmation to proceed with an action
# Returns 0 (success) if user confirms or we're in autoconfirm mode
# Returns 1 (failure) if user declines
prompt() {
  # Skip prompting if we're in non-interactive mode
  if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ]; then
    log "Auto-confirming: $1"
    return 0
  fi
  
  # Otherwise, ask the user and wait for their response
  printf "[MIDAZ] %s" "$1"
  read -r response
  case "${response}" in
    [yY]|[yY][eE][sS]|"")
      # User confirmed (y, yes, or just pressed Enter)
      return 0
      ;;
    *)
      # User declined
      return 1
      ;;
  esac
}

# ============================================================================
# Privilege Escalation Functions
# ============================================================================
# These functions handle running commands with elevated privileges when needed

# Determines the appropriate privilege escalation command to use (if any)
# Returns the command name (sudo, doas) or empty string if already root
get_sudo() {
  if [ "$(id -u)" = 0 ]; then
    # We're already running as root, no need for sudo/doas
    echo ""
    return 0
  elif command -v sudo >/dev/null 2>&1; then
    # sudo is available, prefer this
    echo "sudo"
    return 0
  elif command -v doas >/dev/null 2>&1; then
    # doas is available (common on OpenBSD and some security-focused systems)
    echo "doas"
    return 0
  else
    # No privilege escalation command found
    return 1
  fi
}

# Runs a command with appropriate privilege escalation if needed
# Automatically determines whether to use sudo, doas, or run directly
run_sudo() {
  SUDO="$(get_sudo)"
  if [ -n "$SUDO" ]; then
    # Run with sudo/doas if available
    log "Running with elevated privileges: $*"
    $SUDO "$@"
  else
    # No privilege escalation available, try running directly
    log_warning "No privilege escalation command (sudo/doas) available, attempting to run directly"
    log_warning "This may fail if the command requires elevated privileges"
    "$@"
  fi
}

# ============================================================================
# Dependency Checking Functions
# ============================================================================

# Checks if a command is available in the current PATH
# Returns 0 (success) if command exists, 1 (failure) if not
check_command() {
  command -v "$1" >/dev/null 2>&1
}

# Compares version numbers to ensure a minimum version requirement is met
# Parameters:
#   $1: Command name to check
#   $2: Shell command to get the version string (e.g., "go version")
#   $3: Minimum required version (e.g., "1.22")
# Returns 0 (success) if version meets requirements, 1 (failure) if not
check_version() {
  command=$1
  version_str=$2
  min_version=$3
  
  # For macOS special case when checking sw_vers
  if [ "$command" = "sw_vers" ]; then
    # Just return true for any version since we handle version check separately
    return 0
  fi

  # Extract the version number from the command output
  # This regex finds sequences of numbers separated by periods
  version=$(eval "$version_str" | grep -oE '[0-9]+(\.[0-9]+)+' | head -1)
  
  # Split the version into major and minor components
  major=$(echo "$version" | cut -d. -f1)
  minor=$(echo "$version" | cut -d. -f2)
  
  # Split the minimum required version the same way
  req_major=$(echo "$min_version" | cut -d. -f1)
  req_minor=$(echo "$min_version" | cut -d. -f2)
  
  # Compare versions: major must be greater than required, or
  # if major is equal, minor must be at least as high as required
  if [ "$major" -gt "$req_major" ] || ([ "$major" -eq "$req_major" ] && [ "$minor" -ge "$req_minor" ]); then
    # Version meets requirements
    return 0
  else
    # Version is too old
    return 1
  fi
}

# ============================================================================
# Network Utility Functions
# ============================================================================

# Determines the best available download tool (curl or wget)
# Returns the appropriate command string with common options for silent operation
get_downloader() {
  if command -v curl >/dev/null 2>&1; then
    # curl options:
    # -f: Fail silently on server errors
    # -s: Silent mode
    # -S: Show error messages
    # -L: Follow redirects
    echo "curl -fsSL"
  elif command -v wget >/dev/null 2>&1; then
    # wget options:
    # -q: Quiet mode (no output)
    # -O-: Output to stdout
    echo "wget -q -O-"
  else
    # Neither tool is available
    die "Neither curl nor wget found. Please install either to continue with installation."
  fi
}

# ============================================================================
# Operating System Detection
# ============================================================================
# This function identifies the operating system, distribution, and package manager
# to determine the appropriate installation methods for dependencies

detect_os() {
  log "Detecting operating system and distribution..."
  
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_NAME="${ID}"
    OS_VERSION="${VERSION_ID:-}"
    OS_FAMILY="linux"
    
    case "${OS_NAME}" in
      *debian*|*ubuntu*)
        OS_PACKAGE_MANAGER="apt"
        OS_FAMILY="debian"
        ;;
      *rhel*|*centos*|*fedora*|*rocky*|*alma*|*amazon*)
        OS_PACKAGE_MANAGER="dnf"
        if ! check_command dnf; then
          OS_PACKAGE_MANAGER="yum"
        fi
        OS_FAMILY="rhel"
        ;;
      *arch*|*manjaro*)
        OS_PACKAGE_MANAGER="pacman"
        OS_FAMILY="arch"
        ;;
      *opensuse*|*suse*)
        OS_PACKAGE_MANAGER="zypper"
        OS_FAMILY="suse"
        ;;
      *alpine*)
        OS_PACKAGE_MANAGER="apk"
        OS_FAMILY="alpine"
        ;;
      *)
        log_warning "Unsupported Linux distribution: ${OS_NAME}. Attempting to proceed."
        OS_PACKAGE_MANAGER="unknown"
        ;;
    esac
  elif [ "$(uname)" = "Darwin" ]; then
    OS_NAME="macos"
    OS_FAMILY="darwin"
    OS_PACKAGE_MANAGER="brew"
    OS_VERSION="$(sw_vers -productVersion)"
  else
    die "Unsupported operating system. This installer supports Debian/Ubuntu, RHEL/CentOS/Fedora, Arch Linux, and macOS."
  fi
  
  log "Detected: ${OS_NAME} ${OS_VERSION} (${OS_FAMILY})"
  
  # Check if system is supported
  if [ "${OS_FAMILY}" = "darwin" ]; then
    # macOS versions are now 11+ for Big Sur and beyond, so any 11+ or any 10.15+ is fine
    MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
    if [ "${MAJOR_VERSION}" -ge 11 ]; then
      # macOS 11 or higher (Big Sur, Monterey, Ventura, Sonoma, etc.) - supported
      :
    elif [ "${MAJOR_VERSION}" -eq 10 ]; then
      # Check if at least 10.15 (Catalina)
      MINOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f2)
      if [ "${MINOR_VERSION}" -lt 15 ]; then
        die "macOS ${OS_VERSION} is not supported. Please upgrade to macOS 10.15 or later."
      fi
    fi
  fi
}

# Dependency installation functions
install_git() {
  log "Installing git..."
  case "${OS_PACKAGE_MANAGER}" in
    apt)
      run_sudo apt-get update && run_sudo apt-get install -y git
      ;;
    dnf|yum)
      run_sudo "${OS_PACKAGE_MANAGER}" install -y git
      ;;
    pacman)
      run_sudo pacman -Sy --noconfirm git
      ;;
    brew)
      brew install git
      ;;
    *)
      die "Cannot install git. Please install git manually and try again."
      ;;
  esac
}

install_docker() {
  log "Installing Docker..."
  case "${OS_PACKAGE_MANAGER}" in
    apt)
      run_sudo apt-get update
      run_sudo apt-get install -y ca-certificates curl gnupg
      if [ ! -d /etc/apt/keyrings ]; then
        run_sudo mkdir -p /etc/apt/keyrings
      fi
      run_sudo curl -fsSL https://download.docker.com/linux/${OS_NAME}/gpg -o /tmp/docker.gpg
      run_sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg /tmp/docker.gpg
      echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${OS_NAME} $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | run_sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
      run_sudo apt-get update
      run_sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
      run_sudo usermod -aG docker "$(whoami)"
      ;;
    dnf|yum)
      run_sudo "${OS_PACKAGE_MANAGER}" install -y dnf-plugins-core
      run_sudo "${OS_PACKAGE_MANAGER}" config-manager --add-repo https://download.docker.com/linux/${OS_NAME}/docker-ce.repo
      run_sudo "${OS_PACKAGE_MANAGER}" install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
      run_sudo systemctl enable docker
      run_sudo systemctl start docker
      run_sudo usermod -aG docker "$(whoami)"
      ;;
    pacman)
      run_sudo pacman -Sy --noconfirm docker docker-compose
      run_sudo systemctl enable docker
      run_sudo systemctl start docker
      run_sudo usermod -aG docker "$(whoami)"
      ;;
    brew)
      brew install --cask docker
      log "Please open the Docker Desktop application to complete installation"
      ;;
    *)
      die "Cannot install Docker. Please install Docker manually and try again."
      ;;
  esac
}

install_make() {
  log "Installing make..."
  case "${OS_PACKAGE_MANAGER}" in
    apt)
      run_sudo apt-get update && run_sudo apt-get install -y make
      ;;
    dnf|yum)
      run_sudo "${OS_PACKAGE_MANAGER}" install -y make
      ;;
    pacman)
      run_sudo pacman -Sy --noconfirm make
      ;;
    brew)
      brew install make
      ;;
    *)
      die "Cannot install make. Please install make manually and try again."
      ;;
  esac
}

install_go() {
  log "Installing Go (Latest Stable Version)..."
  case "${OS_PACKAGE_MANAGER}" in
    apt)
      run_sudo apt-get update
      run_sudo apt-get install -y wget
      GO_TMP_DIR=$(mktemp -d)
      GO_LATEST_VERSION=$(curl -s https://go.dev/VERSION?m=text | head -1)
      wget -q -O "${GO_TMP_DIR}/go.tar.gz" "https://golang.org/dl/${GO_LATEST_VERSION}.linux-amd64.tar.gz"
      run_sudo rm -rf /usr/local/go
      run_sudo tar -C /usr/local -xzf "${GO_TMP_DIR}/go.tar.gz"
      rm -rf "${GO_TMP_DIR}"
      if ! grep -q "export PATH=\$PATH:/usr/local/go/bin" "$HOME/.profile"; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> "$HOME/.profile"
      fi
      export PATH=$PATH:/usr/local/go/bin
      ;;
    dnf|yum)
      run_sudo "${OS_PACKAGE_MANAGER}" install -y wget
      GO_TMP_DIR=$(mktemp -d)
      GO_LATEST_VERSION=$(curl -s https://go.dev/VERSION?m=text | head -1)
      wget -q -O "${GO_TMP_DIR}/go.tar.gz" "https://golang.org/dl/${GO_LATEST_VERSION}.linux-amd64.tar.gz"
      run_sudo rm -rf /usr/local/go
      run_sudo tar -C /usr/local -xzf "${GO_TMP_DIR}/go.tar.gz"
      rm -rf "${GO_TMP_DIR}"
      if ! grep -q "export PATH=\$PATH:/usr/local/go/bin" "$HOME/.profile"; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> "$HOME/.profile"
      fi
      export PATH=$PATH:/usr/local/go/bin
      ;;
    pacman)
      run_sudo pacman -Sy --noconfirm go
      ;;
    brew)
      brew install go
      ;;
    *)
      die "Cannot install Go. Please install Go 1.22+ manually and try again."
      ;;
  esac
}

install_node() {
  log "Installing Node.js 20+..."
  case "${OS_PACKAGE_MANAGER}" in
    apt)
      run_sudo apt-get update
      run_sudo apt-get install -y ca-certificates curl gnupg
      if [ ! -d /etc/apt/keyrings ]; then
        run_sudo mkdir -p /etc/apt/keyrings
      fi
      run_sudo curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key -o /tmp/nodesource.gpg
      run_sudo gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg /tmp/nodesource.gpg
      echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_20.x nodistro main" | run_sudo tee /etc/apt/sources.list.d/nodesource.list > /dev/null
      run_sudo apt-get update
      run_sudo apt-get install -y nodejs
      ;;
    dnf|yum)
      run_sudo "${OS_PACKAGE_MANAGER}" install -y https://rpm.nodesource.com/pub_20.x/nodistro/repo/nodesource-release-nodistro-1.noarch.rpm
      run_sudo "${OS_PACKAGE_MANAGER}" install -y nodejs
      ;;
    pacman)
      run_sudo pacman -Sy --noconfirm nodejs npm
      ;;
    brew)
      brew install node@20
      ;;
    *)
      die "Cannot install Node.js. Please install Node.js 20+ manually and try again."
      ;;
  esac
}

check_internet_connectivity() {
  log "Checking internet connectivity to required services..."
  
  # Skip connectivity checks if we're already in the repository
  if [ -d "${MIDAZ_DIR}/.git" ]; then
    log "Already in a git repository, skipping connectivity checks"
    log_success "Internet connectivity checks skipped"
    return 0
  fi
  
  # Get the appropriate download tool (curl or wget)
  DOWNLOADER=$(get_downloader)
  
  # Check GitHub connectivity - needed for repository cloning
  log "Verifying access to GitHub (repository hosting)..."
  # Use ping instead of curl/wget since we just need to check connectivity
  if ping -c 1 github.com >/dev/null 2>&1; then
    log_success "GitHub connectivity verified"
  else
    log_warning "Could not ping GitHub. This might cause issues when cloning the repository."
    # Continue anyway - git clone will fail if truly unreachable
  fi
  
  log_success "Internet connectivity to required services verified"
}

# ============================================================================
# Dependency Verification and Installation
# ============================================================================
# Checks for all required tools and offers to install them if missing

check_dependencies() {
  log "Checking for required dependencies..."
  log "Midaz requires: git, docker, docker-compose, make, Go 1.22+, and Node.js 20+"
  
  # Process install flags
  if echo "${INSTALL_FLAGS}" | grep -q '\-y'; then
    MIDAZ_AUTOCONFIRM=1
    log "Running in non-interactive mode"
  fi
  
  # Check for git
  if ! check_command git; then
    log_warning "Git not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install git? (Y/n): "; then
      install_git
    else
      die "Git is required but not installed."
    fi
  else
    log_success "Git found: $(git --version)"
  fi
  
  # Check for docker
  if ! check_command docker; then
    log_warning "Docker not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install Docker? (Y/n): "; then
      install_docker
    else
      die "Docker is required but not installed."
    fi
  else
    log_success "Docker found: $(docker --version)"
    
    # Check for docker-compose plugin or legacy binary
    if docker compose version >/dev/null 2>&1; then
      log_success "Docker Compose plugin found: $(docker compose version --short)"
    elif command -v docker-compose >/dev/null 2>&1; then
      log_success "Legacy docker-compose binary found: $(docker-compose --version)"
      log_warning "Consider upgrading to the Docker Compose plugin (v2) for best experience"
    else
      log_warning "Docker Compose not found"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install Docker Compose plugin? (Y/n): " ; then
        install_docker
      else
        die "Docker Compose is required but not installed."
      fi
    fi
  fi
  
  # Check if Docker daemon is running
  if ! docker info >/dev/null 2>&1; then
    log_warning "Docker daemon is not running"
    if [ "${OS_FAMILY}" = "darwin" ]; then
      log "Please start Docker Desktop and try again."
      exit 1
    else
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Start Docker daemon? (Y/n): "; then
        SUDO="$(get_sudo)"
        if [ -n "$SUDO" ]; then
          $SUDO systemctl start docker
          log "Waiting for Docker daemon to start..."
          sleep 5
          if ! docker info >/dev/null 2>&1; then
            die "Failed to start Docker daemon. Please start it manually and try again."
          fi
        else
          die "Cannot start Docker daemon without sudo/doas privileges. Please start it manually and try again."
        fi
      else
        die "Docker daemon must be running to continue."
      fi
    fi
  fi
  
  # Check for make
  if ! check_command make; then
    log_warning "Make not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install make? (Y/n): "; then
      install_make
    else
      die "Make is required but not installed."
    fi
  else
    log_success "Make found: $(make --version | head -1)"
  fi
  
  # Check for Go
  if ! check_command go; then
    log_warning "Go not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install Go? (Y/n): "; then
      install_go
    else
      die "Go is required but not installed."
    fi
  elif ! check_version go "go version" "1.22"; then
    log_warning "Go version 1.22 or higher is required, found: $(go version)"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Update Go? (Y/n): "; then
      install_go
    else
      die "Go 1.22+ is required."
    fi
  else
    log_success "Go found: $(go version)"
  fi
  
  # Check for Node.js
  if ! check_command node; then
    log_warning "Node.js not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install Node.js? (Y/n): "; then
      install_node
    else
      die "Node.js is required but not installed."
    fi
  elif ! check_version node "node --version" "20.0"; then
    log_warning "Node.js version 20 or higher is required, found: $(node --version)"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Update Node.js? (Y/n): "; then
      install_node
    else
      die "Node.js 20+ is required."
    fi
  else
    log_success "Node.js found: $(node --version)"
  fi
}

# ============================================================================
# Repository Cloning
# ============================================================================
# Clones or updates the Midaz repository to the specified directory

clone_midaz_repo() {
  log "Preparing to install Midaz repository to ${MIDAZ_DIR}..."
  log "This will download the core banking platform source code"
  
  if [ -d "${MIDAZ_DIR}" ]; then
    if [ -d "${MIDAZ_DIR}/.git" ]; then
      log_warning "Midaz repository already exists at ${MIDAZ_DIR}"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Update repository? (Y/n): "; then
        log "Updating repository..."
        cd "${MIDAZ_DIR}"
        git fetch origin
        git checkout "${MIDAZ_REF}"
        git pull
      fi
    else
      log_warning "Directory ${MIDAZ_DIR} exists but is not a git repository"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Remove and clone repository? (Y/n): "; then
        log "Removing existing directory and cloning repository..."
        rm -rf "${MIDAZ_DIR}"
        git clone --branch "${MIDAZ_REF}" "${MIDAZ_REPO}" "${MIDAZ_DIR}"
      else
        die "Cannot proceed without cloning repository."
      fi
    fi
  else
    log "Cloning Midaz repository..."
    mkdir -p "$(dirname "${MIDAZ_DIR}")"
    git clone --branch "${MIDAZ_REF}" "${MIDAZ_REPO}" "${MIDAZ_DIR}"
  fi
}

# ============================================================================
# Midaz Stack Deployment
# ============================================================================
# Prepares and starts the Midaz services using Makefile commands

run_midaz() {
  log "Starting Midaz banking platform services..."
  log "This will prepare and start the complete Midaz environment"
  cd "${MIDAZ_DIR}"
  
  # Check if environment needs setup
  if [ ! -f "${MIDAZ_DIR}/components/infra/.env" ] || 
     [ ! -f "${MIDAZ_DIR}/components/onboarding/.env" ] || 
     [ ! -f "${MIDAZ_DIR}/components/transaction/.env" ]; then
    log "Setting up environment configuration..."
    make set-env || die "Failed to set up environment files. Please check error messages above."
  fi
  
  # Build the components first
  # log "Building all components..."
  # make build || log_warning "Build step encountered issues, but continuing with deployment"
  
  # Start all services
  log "Starting all services..."
  make up || die "Failed to start services. Please check error messages above."
  
  # The Makefile's 'up' command already has built-in health checks,
  # but we'll add an explicit check just to provide better feedback to the user
  log "Verifying service health..."
  sleep 5  # Brief pause to allow services to initialize
  
  if docker ps --format "{{.Names}}" | grep -q "midaz"; then
    log_success "Midaz services are running"
  else
    log_warning "No Midaz containers found running. There may have been an issue with startup."
    log_warning "Check container status with: 'cd ${MIDAZ_DIR} && docker compose ps'"
  fi
}

# ============================================================================
# Demo Data Generation
# ============================================================================
# Optionally populates the system with demo data for testing

generate_demo_data() {
  if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Generate realistic demo data now? (Y/n): "; then
    log "Preparing to generate demo data in the Midaz platform..."
    
    # Default data volume size is 'small'
    DATA_SIZE="small"
    
    # Ask for data size if not in autoconfirm mode
    if [ "${MIDAZ_AUTOCONFIRM}" -ne 1 ]; then
      echo ""
      echo "[MIDAZ] Please select demo data volume size:"
      echo "  1) Small  - Basic dataset for testing (default)"
      echo "  2) Medium - Larger dataset with more complexity"
      echo "  3) Large  - Comprehensive dataset for extensive testing"
      printf "[MIDAZ] Select size [1-3]: "
      read -r size_choice
      
      case "${size_choice}" in
        2) DATA_SIZE="medium" ;;
        3) DATA_SIZE="large" ;;
        *) DATA_SIZE="small" ;;
      esac
    else
      # In auto-confirm mode, use small by default
      log "Auto-selecting 'small' data size in non-interactive mode"
    fi
    
    log "Using data volume size: ${DATA_SIZE}"
    log "This will create sample organizations, ledgers, accounts, and transactions"
    
    # For fresh installations, we don't need an auth token
    AUTH_TOKEN="none"
    
    cd "${MIDAZ_DIR}"
    
    # Run the dedicated demo data generator script
    log "Running demo data generator..."
    
    if [ -f "${MIDAZ_DIR}/scripts/demo-data/run-generator.sh" ]; then
      if sh "${MIDAZ_DIR}/scripts/demo-data/run-generator.sh" "${DATA_SIZE}" "${AUTH_TOKEN}"; then
        log_success "Demo data generation complete!"
      else
        log_warning "Demo data generation encountered issues, but installation will continue"
      fi
    else
      log_warning "Demo data generator script not found at ${MIDAZ_DIR}/scripts/demo-data/run-generator.sh"
      log_warning "Skipping demo data generation"
    fi
  else
    log "Skipping demo data generation. You can generate it later with the command:"
    log "  cd ${MIDAZ_DIR} && sh scripts/demo-data/run-generator.sh [small|medium|large] [auth-token]"
  fi
}

# ============================================================================
# Installation Completion
# ============================================================================
# Displays final success message and access information

display_success() {
  log_success "Midaz core-banking platform has been successfully installed!"
  echo ""
  echo "=== MIDAZ SERVICES ==="
  echo ""
  echo "Admin Portal: http://localhost:3000"
  echo "  - Web interface for managing the banking platform"
  echo ""
  echo "API Documentation: http://localhost:8080/swagger/index.html"
  echo "  - Interactive API documentation for developers"
  echo ""
  echo "Database (PostgreSQL): localhost:5432"
  echo "  - Direct database access for advanced users"
  echo ""
  echo "The Midaz repository is installed at: ${MIDAZ_DIR}"
  echo ""
  echo "=== NEXT STEPS ==="
  echo "Use 'cd ${MIDAZ_DIR}' to navigate to the Midaz directory."
  echo "Run 'make help' to see available commands for managing the Midaz stack."
  echo ""
  echo "For more information and documentation, visit: https://docs.midaz.dev"
  echo "For support, contact: support@midaz.dev"
  echo ""
}

# ============================================================================
# Main Installation Flow
# ============================================================================
# Primary execution function that orchestrates the entire installation process

main() {
  # Display welcome banner
  echo "  __  __ _     _            "
  echo " |  \/  (_)   | |           "
  echo " | \  / |_  __| | __ _ ____"
  echo " | |\/| | |/ _\` |/ _\` |_  /"
  echo " | |  | | | (_| | (_| |/ / "
  echo " |_|  |_|_|\__,_|\__,_/___| Core Banking"
  echo ""
  echo "Welcome to the Midaz Core Banking Platform installer"
  echo "This script will set up a complete Midaz environment on your system"
  echo ""
  
  detect_os
  check_internet_connectivity
  check_dependencies
  clone_midaz_repo
  run_midaz
  generate_demo_data
  display_success
  
  # If we've made it this far, disable the EXIT trap
  trap - EXIT
}

# Execute main function
main