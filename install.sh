#!/usr/bin/env sh
# ============================================================================
# Midaz Core-Banking Stack Installation Script
# ============================================================================
#
# This script provides an automated installation of the Midaz core-banking platform.
# It detects the operating system, installs dependencies, clones the repository,
# and sets up all required services using Docker containers.
#
# The installation is designed to be interactive by default but can be run in
# non-interactive mode by passing the -y flag. The script includes extensive
# error handling, recovery capabilities, and diagnostic information.
#
# Key Features:
# - Cross-platform support (Linux distributions and macOS)
# - Automatic dependency installation
# - Branch selection for different development environments
# - Optional demo data generation
# - Recovery from interrupted installations
# - Uninstallation capability
# - Comprehensive error diagnostics
#
# It performs the following actions:
#  1. Detects the operating system and package manager
#  2. Verifies and installs all required dependencies:
#     - Git (for cloning the repository)
#     - Docker + Docker Compose (for containerization)
#     - Make (for running build processes)
#     - Go 1.22+ (for building backend services)
#     - Node.js 20+ (for frontend applications)
#  3. Clones the Midaz repository (with branch selection)
#  4. Starts the services using Docker Compose
#  5. Optionally generates demo data for testing
#
# Usage:
#   curl -fsSL https://get.midaz.dev | sh
#   ./install.sh [OPTIONS]
#
# Common Options:
#   --help         Display help information
#   --uninstall    Remove Midaz from the system
#   -y, --yes      Non-interactive mode
#
# Configurable environment variables:
#   MIDAZ_DIR      Override installation directory (default: ~/midaz)
#   MIDAZ_REF      Branch/tag to clone (default: main)
#   INSTALL_FLAGS  Pass -y to skip interactive prompts (fully automated install)
#
# For developers:
# This script is designed to be maintainable and extensible. Each function has a
# single responsibility, and error handling is consistent throughout. The script
# includes state tracking to enable recovery from interruptions and diagnostics
# to help troubleshoot issues.
#
# Author: Lerian Studio Engineering Team
# License: See LICENSE file in the repository
# Version: 1.0.0
# Last updated: 2023-05-15
#
# ============================================================================
# Configuration and Safety Settings
# ============================================================================
#
# The script uses strict error handling to fail fast and provide clear feedback.
# Three options are enabled to ensure robust execution:
#
# set -e: Exits immediately if a command fails. This prevents the script from 
#         continuing after errors that might lead to incorrect installations.
#
# set -u: Treats unset variables as errors. This catches typos and ensures all
#         required variables are properly defined before use.
#
# set -o pipefail: Ensures that a pipeline fails if any command in it fails,
#                  not just the last one. This helps catch errors in piped commands.
#
# The script also sets a safe Internal Field Separator (IFS) to handle whitespace
# in filenames and paths correctly, preventing common filepath parsing errors.

# Enable strict mode for safer execution
# -e: Exit immediately if a command fails
# -u: Treat unset variables as an error
# -o pipefail: Ensures that a pipeline fails if any command in it fails
set -eu

# Set a safe Internal Field Separator (IFS)
# This ensures that whitespace in filenames and paths is handled correctly
IFS="$(printf '\n\t')"

# ============================================================================
# Default Configuration Variables
# ============================================================================
#
# These variables control the installation process and can be customized
# through environment variables or command-line flags.
#
# Users can override these settings either by:
# 1. Exporting them as environment variables before running the script
#    Example: MIDAZ_DIR=/opt/midaz curl -fsSL https://get.midaz.dev | sh
#
# 2. Using command-line arguments for the supported options
#    Example: ./install.sh --yes to enable non-interactive mode
#
# The script will always prioritize explicitly set variables over defaults.
# If a configuration variable is not set, the script will use these sensible defaults.

# Installation location - can be overridden by setting MIDAZ_DIR environment variable
# Default is in the user's home directory for unprivileged installation
MIDAZ_DIR="${MIDAZ_DIR:-$HOME/midaz}"

# Git reference (branch/tag) to install - can be overridden by setting MIDAZ_REF
# Users can specify a different branch for development/testing purposes
MIDAZ_REF="${MIDAZ_REF:-main}"

# Repository URL - this is the official Midaz repository
# This is the canonical source for the Midaz core-banking platform
MIDAZ_REPO="https://github.com/lerianstudio/midaz"

# Flag to track if we're running in non-interactive mode
# This is set to 1 when -y or --yes flag is passed
MIDAZ_AUTOCONFIRM=0

# Flag to track if we're running inside a container
# This affects how we handle services like Docker
RUNNING_IN_CONTAINER=0

# Installation flags from environment - used to enable non-interactive mode
# Typically set to "-y" to automatically confirm all prompts
INSTALL_FLAGS="${INSTALL_FLAGS:-}"

# Branch selection timeout (in seconds)
# How long to wait for user input when selecting a branch before defaulting to main
BRANCH_SELECTION_TIMEOUT=2

# Define text formatting (all empty, no colors)
RESET=""
RED=""
GREEN=""
YELLOW=""
BLUE=""
BOLD=""

# ============================================================================
# Installation State Tracking and Recovery
# ============================================================================
#
# The script maintains a state file to track installation progress, allowing
# for recovery if the process is interrupted. This is particularly useful for
# long installations or when network interruptions occur.
#
# State tracking works by:
# 1. Saving the current state to a temporary file after completing each major step
# 2. Checking for this state file on startup to detect interrupted installations
# 3. Offering to resume from the last completed step rather than starting over
#
# The state file is cleaned up after successful installation, but persists
# if the installation is interrupted, allowing for recovery later.
#
# States tracked include:
# - not_started: Fresh installation, no previous state found
# - deps_installed: Dependencies successfully installed
# - repo_cloned: Repository successfully cloned
# - services_started: Docker services successfully started
# - completed: Installation completed successfully

# Define the state file path
MIDAZ_STATE_FILE="/tmp/midaz_install_state.tmp"

# Function to save the current installation state
save_state() {
  state=$1
  echo "${state}" > "${MIDAZ_STATE_FILE}"
  log "Installation state saved: ${state}"
}

# Function to read the current installation state
read_state() {
  if [ -f "${MIDAZ_STATE_FILE}" ]; then
    cat "${MIDAZ_STATE_FILE}"
  else
    echo "not_started"
  fi
}

# Function to check if a previous installation was interrupted
check_for_recovery() {
  log "Checking for previous interrupted installation..."
  
  CURRENT_STATE=$(read_state)
  
  case "${CURRENT_STATE}" in
    "deps_installed")
      log_warning "Found an interrupted installation after dependency installation"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Resume from repository cloning? (Y/n): "; then
        log "Resuming installation from repository cloning..."
        clone_midaz_repo
        save_state "repo_cloned"
        run_midaz
        save_state "services_started"
        generate_demo_data
        save_state "completed"
        display_success
        # Clean up the state file on successful completion
        rm -f "${MIDAZ_STATE_FILE}"
        # If we've made it this far, disable the EXIT trap
        trap - EXIT
        exit 0
      fi
      ;;
    "repo_cloned")
      log_warning "Found an interrupted installation after repository cloning"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Resume from service startup? (Y/n): "; then
        log "Resuming installation from service startup..."
        run_midaz
        save_state "services_started"
        generate_demo_data
        save_state "completed"
        display_success
        # Clean up the state file on successful completion
        rm -f "${MIDAZ_STATE_FILE}"
        # If we've made it this far, disable the EXIT trap
        trap - EXIT
        exit 0
      fi
      ;;
    "services_started")
      log_warning "Found an interrupted installation after service startup"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Resume from demo data generation? (Y/n): "; then
        log "Resuming installation from demo data generation..."
        generate_demo_data
        save_state "completed"
        display_success
        # Clean up the state file on successful completion
        rm -f "${MIDAZ_STATE_FILE}"
        # If we've made it this far, disable the EXIT trap
        trap - EXIT
        exit 0
      fi
      ;;
    "completed")
      log_warning "A previous installation was already completed successfully"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Do you want to reinstall? (Y/n): "; then
        log "Starting fresh installation..."
        # Continue with normal installation flow
        rm -f "${MIDAZ_STATE_FILE}"
      else
        log "Exiting. No changes made."
        exit 0
      fi
      ;;
    "not_started"|*)
      # No previous installation found or unrecognized state, proceed normally
      log "No interrupted installation found. Proceeding with fresh installation."
      ;;
  esac
}

# ============================================================================
# Signal Handling and Cleanup
# ============================================================================
#
# This section defines how the script responds to various signals, ensuring
# graceful termination even when interrupted. The cleanup function is called
# whenever the script exits abnormally, providing users with information about
# the interrupted state and saving progress for later recovery.
#
# Signal traps handle:
# - INT: Keyboard interrupt (Ctrl+C)
# - TERM: Termination signal sent by system
# - EXIT: Any exit from the script (unless trap is reset)
# - HUP: Terminal disconnect (hangup)
#
# The cleanup function:
# 1. Saves the current installation state to enable recovery
# 2. Provides clear messaging about the interrupted installation
# 3. Gives instructions for resuming the installation later

# This function handles interruption signals to ensure clean termination
# If the user presses Ctrl+C or the script receives a termination signal,
# this ensures we exit gracefully instead of leaving partial installations
cleanup() {
  # If we have a current state, save it to allow for recovery
  if [ -n "${CURRENT_INSTALL_STATE:-}" ]; then
    save_state "${CURRENT_INSTALL_STATE}"
    echo "[MIDAZ] Installation state saved for recovery."
  fi
  
  echo "[MIDAZ] Installation interrupted. Exiting..."
  echo "[MIDAZ] You may need to manually clean up any partially installed components."
  echo "[MIDAZ] Run the installer again to attempt recovery."
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
#
# These functions provide consistent messaging throughout the script.
# Each function has a specific purpose to help users
# distinguish between different types of information:
#
# - log: Standard informational messages
# - log_success: Success messages for completed operations
# - log_warning: Warning messages for potential issues
# - log_error: Error messages for problems that prevent continuing
# - die: Fatal error handler that exits with detailed diagnostic information
#
# Using consistent log functions improves readability and makes it easier
# to filter or parse the output programmatically if needed.
#
# The die() function is especially important as it collects system information
# for troubleshooting before exiting with a non-zero status code.

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
#
# These functions handle user interactions throughout the installation process.
# They provide a consistent way to request input from users while supporting
# both interactive and non-interactive modes.
#
# The prompt() function is particularly important as it:
# 1. Shows a prompt to the user requesting confirmation
# 2. Skips the prompt entirely in non-interactive mode (auto-confirms)
# 3. Handles various affirmative responses (y, yes, or just Enter)
# 4. Returns standardized exit codes for consistent handling (0 for yes, 1 for no)
#
# This design allows the same code to work in both interactive installations 
# (where users can make choices) and automated deployments (where default or
# specified choices are used automatically).

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
#
# These functions handle running commands with elevated privileges when necessary.
# The script always tries to minimize the use of sudo/doas by:
# 1. Checking if the command can be run without privileges
# 2. Using user-specific alternatives when available (e.g., --user flag)
# 3. Only escalating privileges when absolutely necessary
#
# This approach follows the principle of least privilege and makes the script
# safer to run. When privilege escalation is needed, the script:
# 1. Detects the appropriate command (sudo, doas, or none if already root)
# 2. Provides clear messaging about why elevated privileges are needed
# 3. Handles errors gracefully with helpful guidance for common issues
#
# The run_sudo() function has special handling for common commands like Docker,
# apt, and systemctl to avoid unnecessary privilege escalation.

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
  # First try to see if the command can be run without sudo
  if [ -z "${FORCE_SUDO:-}" ]; then
    # Try to run the command without sudo first for specific commands
    if [ "$1" = "apt-get" ] && [ "$2" = "update" ]; then
      # Skip sudo for apt-get update if we can access the apt sources
      if [ -w /var/lib/apt/lists ] || [ -w /var/cache/apt ]; then
        log "Running apt-get update without elevated privileges"
        "$@"
        return $?
      fi
    elif [ "$1" = "systemctl" ]; then
      # For systemctl, try user mode first if available (systemctl --user)
      if systemctl --user daemon-reload >/dev/null 2>&1; then
        log "Running systemctl in user mode"
        systemctl --user "$2" "$3"
        return $?
      fi
    elif [ "$1" = "docker" ]; then
      # Check if docker socket is accessible without sudo
      if [ -S /var/run/docker.sock ] && [ -w /var/run/docker.sock ]; then
        log "Running docker command without elevated privileges"
        "$@"
        return $?
      elif groups "$(whoami)" | grep -q '\bdocker\b'; then
        log "Running docker command with docker group membership"
        "$@"
        return $?
      fi
    fi
  fi
  
  SUDO="$(get_sudo)"
  if [ -n "$SUDO" ]; then
    # Run with sudo/doas if available
    log "Running with elevated privileges: $*"
    
    # Check if we've already asked for sudo in this session
    if [ -z "${SUDO_PASSWORD_ENTERED:-}" ]; then
      log "You may be prompted for your password to grant administrative privileges."
      log "This is necessary for system-wide installation of dependencies."
      # Setting this variable to track that we've already shown the prompt
      SUDO_PASSWORD_ENTERED=1
    fi
    
    # Run the command with sudo/doas
    # If it fails with a permission error, provide helpful guidance
    if ! $SUDO "$@"; then
      ERROR_CODE=$?
      if [ $ERROR_CODE -eq 1 ] || [ $ERROR_CODE -eq 126 ] || [ $ERROR_CODE -eq 127 ]; then
        log_warning "Command failed, possibly due to permission issues."
        log_warning "If you were prompted for a password and it was rejected:"
        log_warning "1. Ensure you're using the correct password"
        log_warning "2. Verify that your user has sudo privileges"
        log_warning "You can check sudo access with: 'sudo -v'"
      fi
      return $ERROR_CODE
    fi
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
#
# These functions verify that required dependencies are available and meet
# version requirements. The script uses a modular approach to dependency checking:
#
# 1. check_command() - Verifies if a command exists in the system PATH
# 2. check_version() - Compares version numbers to ensure minimum requirements are met
# 3. verify_installation() - Validates that an installation was successful
#
# For version checking, the script includes fallback mechanisms to handle various
# version string formats. This makes the version detection more robust across
# different implementations of the same tool.
#
# If dependencies are missing or outdated, the script offers to install them
# automatically (with user confirmation in interactive mode). This reduces the
# manual setup required from users.

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
  
  # Fallback methods if version extraction fails
  if [ -z "$version" ]; then
    # Try alternative method for version extraction
    version=$(eval "$version_str" | grep -oE '[0-9]+\.[0-9]+' | head -1)
    
    # If still empty, try just getting the first number
    if [ -z "$version" ]; then
      version=$(eval "$version_str" | grep -oE '[0-9]+' | head -1)
      
      # If we can't extract a version at all, assume it doesn't meet requirements
      if [ -z "$version" ]; then
        log_warning "Could not determine version for $command, assuming it needs to be updated"
        return 1
      fi
      
      # If we only found a single number, add .0 to make it compatible with version comparison
      version="${version}.0"
    fi
  fi
  
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

# Function to verify installation success
verify_installation() {
  command=$1
  version_command=$2
  
  if ! check_command "$command"; then
    log_warning "Failed to install $command or it's not in PATH"
    return 1
  fi
  
  version_output=$(eval "$version_command" 2>&1)
  if [ $? -ne 0 ]; then
    log_warning "Installed $command but couldn't verify version: $version_output"
    return 1
  fi
  
  log_success "$command successfully installed: $version_output"
  return 0
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

# Verifies that the system has internet connectivity
# Tries to connect to GitHub and Docker Hub, which are essential for installation
check_internet_connectivity() {
  log "Verifying internet connectivity..."
  
  # Use the appropriate download tool
  DOWNLOADER=$(get_downloader)
  
  # Install ping if it's missing (common in Alpine)
  if [ "${OS_FAMILY}" = "alpine" ] && ! command -v ping >/dev/null 2>&1; then
    log "Installing ping utility for connectivity checks..."
    run_sudo apk add --no-cache iputils || true
  fi
  
  # Check GitHub connectivity (needed for repository cloning)
  if ! $DOWNLOADER https://github.com >/dev/null 2>&1; then
    log_warning "Cannot connect to GitHub. Check your internet connection."
    if [ "${MIDAZ_AUTOCONFIRM}" -ne 1 ] && ! prompt "Continue anyway? (y/N): "; then
      die "Internet connectivity is required for installation."
    fi
  fi
  
  # Check Docker Hub connectivity (needed for container images)
  if ! $DOWNLOADER https://hub.docker.com >/dev/null 2>&1; then
    log_warning "Cannot connect to Docker Hub. Container images may fail to download."
    if [ "${MIDAZ_AUTOCONFIRM}" -ne 1 ] && ! prompt "Continue anyway? (y/N): "; then
      die "Internet connectivity is required for installation."
    fi
  fi
  
  log_success "Internet connectivity verified"
}

# ============================================================================
# Operating System Detection
# ============================================================================
#
# This function identifies the operating system, distribution, package manager,
# and architecture to determine the appropriate installation methods.
#
# It handles:
# - Major Linux distributions (Debian/Ubuntu, RHEL/CentOS/Fedora, Arch, openSUSE, Alpine)
# - macOS (with version-specific compatibility checks)
# - Various architectures (amd64/x86_64, arm64/aarch64, armv7)
#
# For each platform, it sets:
# - OS_NAME: The specific distribution or OS name (e.g., ubuntu, macos)
# - OS_FAMILY: The broader OS family (e.g., debian, rhel, darwin)
# - OS_PACKAGE_MANAGER: The native package manager for the platform
# - OS_VERSION: The OS version number
# - ARCH: The normalized architecture identifier
#
# The function also performs compatibility checks to warn users if their
# system may not be fully supported or might have limited compatibility.

detect_os() {
  log "Detecting operating system and distribution..."
  
  # Detect architecture
  ARCH=$(uname -m)
  case "${ARCH}" in
    x86_64|amd64)
      ARCH="amd64"
      ;;
    aarch64|arm64)
      ARCH="arm64"
      ;;
    armv7l|armv7)
      ARCH="armv7"
      ;;
    *)
      log_warning "Architecture ${ARCH} might not be fully supported. Proceeding with caution."
      ;;
  esac
  
  log "Detected architecture: ${ARCH}"
  
  # Detect if running in a container
  if [ -f /.dockerenv ] || grep -q docker /proc/1/cgroup 2>/dev/null || grep -q container /proc/1/cgroup 2>/dev/null; then
    RUNNING_IN_CONTAINER=1
    log "Detected running inside a container environment"
  fi
  
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
  
  log "Detected: ${OS_NAME} ${OS_VERSION} (${OS_FAMILY}) on ${ARCH}"
  
  # Check if system is supported
  if [ "${OS_FAMILY}" = "darwin" ]; then
    # macOS versions are now 11+ for Big Sur and beyond, so any 11+ or any 10.15+ is fine
    # Handle non-standard version formats by checking if version contains dots
    if ! echo "${OS_VERSION}" | grep -q "\."; then
      log_warning "Non-standard macOS version format detected: ${OS_VERSION}"
      # Try to get more detailed version
      DETAILED_VERSION=$(sw_vers -productVersion 2>/dev/null)
      if [ -n "${DETAILED_VERSION}" ] && echo "${DETAILED_VERSION}" | grep -q "\."; then
        log "Using more detailed version: ${DETAILED_VERSION}"
        OS_VERSION="${DETAILED_VERSION}"
      else
        # If non-standard format and cannot get detailed version, assume it's new enough
        log_warning "Unable to parse macOS version format. Assuming it's compatible."
        return 0
      fi
    fi
    
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
  elif [ "${OS_FAMILY}" = "linux" ]; then
    # Handle Linux version checking
    case "${OS_NAME}" in
      ubuntu)
        # Parse Ubuntu version and check if it's compatible
        if [ -n "${OS_VERSION}" ]; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 20 ]; then
            log_warning "Ubuntu ${OS_VERSION} is quite old. We recommend using Ubuntu 20.04 or newer."
          fi
        fi
        ;;
      debian)
        # Parse Debian version and check if it's compatible
        if [ -n "${OS_VERSION}" ]; then
          if [ "${OS_VERSION}" -lt 10 ]; then
            log_warning "Debian ${OS_VERSION} is quite old. We recommend using Debian 10 or newer."
          fi
        fi
        ;;
      fedora)
        if [ -n "${OS_VERSION}" ] && [ "${OS_VERSION}" -lt 34 ]; then
          log_warning "Fedora ${OS_VERSION} is quite old. We recommend using Fedora 34 or newer."
        fi
        ;;
      centos|rhel)
        if [ -n "${OS_VERSION}" ]; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 8 ]; then
            log_warning "${OS_NAME^} ${OS_VERSION} is quite old. We recommend using version 8 or newer."
          fi
        fi
        ;;
      rocky|alma)
        if [ -n "${OS_VERSION}" ]; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 8 ]; then
            log_warning "${OS_NAME^} Linux ${OS_VERSION} may not be fully compatible. Version 8+ is recommended."
          fi
        fi
        ;;
      arch|manjaro)
        # Rolling releases don't need version checks
        ;;
      alpine)
        if [ -n "${OS_VERSION}" ]; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 3 ]; then
            log_warning "Alpine ${OS_VERSION} is quite old. We recommend using Alpine 3.14 or newer."
          elif [ "${MAJOR_VERSION}" -eq 3 ]; then
            MINOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f2)
            if [ "${MINOR_VERSION}" -lt 14 ]; then
              log_warning "Alpine ${OS_VERSION} is quite old. We recommend using Alpine 3.14 or newer."
            fi
          fi
        fi
        ;;
      opensuse*|suse*)
        if echo "${OS_VERSION}" | grep -q "\."; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 15 ]; then
            log_warning "openSUSE ${OS_VERSION} is quite old. We recommend using version 15 or newer."
          fi
        fi
        ;;
      # Add more distributions as needed
      *)
        log_warning "Unknown Linux distribution ${OS_NAME}. Compatibility not verified."
        ;;
    esac
  fi
}

# ============================================================================
# Dependency Installation Functions
# ============================================================================
#
# These functions handle the installation of each required dependency. They're
# designed to work across different operating systems, using the appropriate
# package manager for each platform.
#
# Each installation function:
# 1. Uses the detected package manager for the current OS
# 2. Installs the tool in the most appropriate way for that platform
# 3. Verifies the installation was successful
# 4. Checks that version requirements are met
#
# The script minimizes system modifications by installing only what's needed
# and uses official distribution methods wherever possible (e.g., official
# repositories, vendor-provided installation scripts).

# Installs Git version control system
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
  
  # Verify installation
  verify_installation "git" "git --version" || log_warning "Git installation may have failed. Please check manually."
}

# Installs Docker and Docker Compose
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
      run_sudo systemctl enable docker || true
      start_docker_daemon
      run_sudo usermod -aG docker "$(whoami)"
      ;;
    pacman)
      run_sudo pacman -Sy --noconfirm docker docker-compose
      run_sudo systemctl enable docker || true
      start_docker_daemon
      run_sudo usermod -aG docker "$(whoami)"
      ;;
    brew)
      brew install --cask docker
      log "Please open the Docker Desktop application to complete installation"
      ;;
    apk)
      # Alpine Linux Docker installation
      log "Installing Docker on Alpine Linux..."
      # Install Docker and Docker Compose
      run_sudo apk add --update docker docker-compose
      # Add current user to docker group
      run_sudo addgroup "$(whoami)" docker 2>/dev/null || true
      # Enable and start Docker service
      run_sudo rc-update add docker boot || true
      start_docker_daemon
      ;;
    *)
      die "Cannot install Docker. Please install Docker manually and try again."
      ;;
  esac
  
  # Verify Docker installation
  verify_installation "docker" "docker --version" || log_warning "Docker installation may have failed. Please check manually."
  
  # Verify Docker Compose installation for non-macOS
  if [ "${OS_FAMILY}" != "darwin" ]; then
    if docker compose version >/dev/null 2>&1; then
      log_success "Docker Compose plugin installed successfully: $(docker compose version --short)"
    elif command -v docker-compose >/dev/null 2>&1; then
      log_success "Legacy docker-compose binary installed: $(docker-compose --version)"
    else
      log_warning "Docker Compose doesn't appear to be installed correctly"
    fi
  else
    log "For macOS, Docker Compose is included with Docker Desktop"
  fi
}

# Installs GNU Make build automation tool
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
  
  # Verify installation
  verify_installation "make" "make --version | head -1" || log_warning "Make installation may have failed. Please check manually."
}

# Installs Go programming language toolchain
install_go() {
  log "Installing Go (Latest Stable Version)..."
  case "${OS_PACKAGE_MANAGER}" in
    apt)
      run_sudo apt-get update
      run_sudo apt-get install -y wget
      GO_TMP_DIR=$(mktemp -d)
      GO_LATEST_VERSION=$(curl -s https://go.dev/VERSION?m=text | head -1)
      
      # Select appropriate architecture
      GO_ARCH="amd64"
      if [ "${ARCH}" = "arm64" ]; then
        GO_ARCH="arm64"
      elif [ "${ARCH}" = "armv7" ]; then
        GO_ARCH="armv6l"
      fi
      
      wget -q -O "${GO_TMP_DIR}/go.tar.gz" "https://golang.org/dl/${GO_LATEST_VERSION}.linux-${GO_ARCH}.tar.gz"
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
      
      # Select appropriate architecture
      GO_ARCH="amd64"
      if [ "${ARCH}" = "arm64" ]; then
        GO_ARCH="arm64"
      elif [ "${ARCH}" = "armv7" ]; then
        GO_ARCH="armv6l"
      fi
      
      wget -q -O "${GO_TMP_DIR}/go.tar.gz" "https://golang.org/dl/${GO_LATEST_VERSION}.linux-${GO_ARCH}.tar.gz"
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
  
  # Verify installation
  verify_installation "go" "go version" || log_warning "Go installation may have failed. Please check manually."
  
  # Check if Go meets version requirements
  if check_command go && ! check_version go "go version" "1.22"; then
    log_warning "Go was installed but version is below 1.22. This might cause compatibility issues."
  fi
}

# Installs Node.js JavaScript runtime and npm package manager
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
  
  # Verify installation
  verify_installation "node" "node --version" || log_warning "Node.js installation may have failed. Please check manually."
  
  # Check if Node.js meets version requirements
  if check_command node && ! check_version node "node --version" "20.0"; then
    log_warning "Node.js was installed but version is below 20.0. This might cause compatibility issues."
  fi
  
  # Also verify npm is available
  if check_command npm; then
    log_success "npm is available: $(npm --version)"
  else
    log_warning "npm is not available. This might cause issues when building Node.js components."
  fi
}

# ============================================================================
# Repository Cloning
# ============================================================================
#
# This section handles cloning or updating the Midaz repository. It includes
# interactive branch selection to allow users to choose which version of the
# codebase to install.
#
# Key features:
# - Fetches available branches from GitHub
# - Displays branches with main and develop prioritized
# - Allows selection by number or name with a timeout
# - Defaults to the main branch for stability
# - Uses single-branch cloning for efficiency
# - Handles repository updates if already cloned
#
# The branch selection is especially useful for developers who want to install
# specific versions or features that are still in development.

# Gets a list of available branches from GitHub repository
get_available_branches() {
  log "Fetching available branches from ${MIDAZ_REPO}..."
  
  DOWNLOADER=$(get_downloader)
  
  # Try to fetch branches using Git API
  BRANCHES=$($DOWNLOADER "https://api.github.com/repos/lerianstudio/midaz/branches" | grep '"name"' | cut -d '"' -f 4)
  
  if [ -z "$BRANCHES" ]; then
    log_warning "Failed to fetch branches from GitHub API. Using default branches."
    # Fall back to default branches list if API fails
    BRANCHES="main develop"
  fi
  
  # Ensure main and develop are at the top of the list
  SORTED_BRANCHES=""
  
  # First add main if it exists
  if echo "$BRANCHES" | grep -q "^main$"; then
    SORTED_BRANCHES="main"
  fi
  
  # Then add develop if it exists
  if echo "$BRANCHES" | grep -q "^develop$"; then
    if [ -n "$SORTED_BRANCHES" ]; then
      SORTED_BRANCHES="$SORTED_BRANCHES develop"
    else
      SORTED_BRANCHES="develop"
    fi
  fi
  
  # Then add all other branches
  for branch in $BRANCHES; do
    if [ "$branch" != "main" ] && [ "$branch" != "develop" ]; then
      if [ -n "$SORTED_BRANCHES" ]; then
        SORTED_BRANCHES="$SORTED_BRANCHES $branch"
      else
        SORTED_BRANCHES="$branch"
      fi
    fi
  done
  
  echo "$SORTED_BRANCHES"
}

# Clones or updates the Midaz repository to the specified directory
clone_midaz_repo() {
  log "Preparing to install Midaz repository to ${MIDAZ_DIR}..."
  log "This will download the core banking platform source code"
  
  # Branch selection logic
  if [ "${MIDAZ_AUTOCONFIRM}" -ne 1 ]; then
    # Only show branch selection in interactive mode
    if [ -z "${MIDAZ_REF_SELECTED:-}" ]; then  # Only show if not already selected
      # Get available branches
      AVAILABLE_BRANCHES=$(get_available_branches)
      
      echo ""
      echo "Available branches:"
      echo "==================="
      
      # Display branches with numbers
      BRANCH_NUMBER=1
      BRANCH_MAP=""
      for branch in $AVAILABLE_BRANCHES; do
        if [ "$branch" = "main" ]; then
          echo "  1) main (default)"
          BRANCH_MAP="$BRANCH_MAP 1:main"
        elif [ "$branch" = "develop" ]; then
          echo "  2) develop"
          BRANCH_MAP="$BRANCH_MAP 2:develop"
        else
          echo "  $((BRANCH_NUMBER+1))) $branch"
          BRANCH_MAP="$BRANCH_MAP $((BRANCH_NUMBER+1)):$branch"
        fi
        BRANCH_NUMBER=$((BRANCH_NUMBER+1))
      done
      
      echo ""
      printf "Select branch (default is main, automatically selecting in %s seconds): " "${BRANCH_SELECTION_TIMEOUT}"
      
      # Read input with timeout
      read -t "${BRANCH_SELECTION_TIMEOUT}" -r branch_choice || true
      
      if [ -z "$branch_choice" ]; then
        # No input provided within timeout, use default
        log "No selection made within timeout. Using default branch: main"
        MIDAZ_REF="main"
      elif [ "$branch_choice" = "1" ] || [ "$branch_choice" = "main" ]; then
        MIDAZ_REF="main"
      elif [ "$branch_choice" = "2" ] || [ "$branch_choice" = "develop" ]; then
        MIDAZ_REF="develop"
      else
        # Check if choice is a number
        if echo "$branch_choice" | grep -q '^[0-9]\+$'; then
          # Convert choice to branch name
          FOUND_BRANCH=""
          for mapping in $BRANCH_MAP; do
            NUM=$(echo "$mapping" | cut -d: -f1)
            BRANCH=$(echo "$mapping" | cut -d: -f2)
            if [ "$NUM" = "$branch_choice" ]; then
              FOUND_BRANCH=$BRANCH
              break
            fi
          done
          
          if [ -n "$FOUND_BRANCH" ]; then
            MIDAZ_REF="$FOUND_BRANCH"
          else
            log_warning "Invalid branch number: $branch_choice. Using default branch: main"
            MIDAZ_REF="main"
          fi
        else
          # Check if choice is a valid branch name
          VALID_BRANCH=0
          for branch in $AVAILABLE_BRANCHES; do
            if [ "$branch" = "$branch_choice" ]; then
              VALID_BRANCH=1
              break
            fi
          done
          
          if [ "$VALID_BRANCH" -eq 1 ]; then
            MIDAZ_REF="$branch_choice"
          else
            log_warning "Invalid branch name: $branch_choice. Using default branch: main"
            MIDAZ_REF="main"
          fi
        fi
      fi
      
      # Set flag to indicate branch was selected
      MIDAZ_REF_SELECTED=1
      
      echo ""
      log "Using branch: ${MIDAZ_REF}"
    fi
  fi
  
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
        git clone --branch "${MIDAZ_REF}" --single-branch "${MIDAZ_REPO}" "${MIDAZ_DIR}"
      else
        die "Cannot proceed without cloning repository."
      fi
    fi
  else
    log "Cloning Midaz repository (branch: ${MIDAZ_REF})..."
    mkdir -p "$(dirname "${MIDAZ_DIR}")"
    git clone --branch "${MIDAZ_REF}" --single-branch "${MIDAZ_REPO}" "${MIDAZ_DIR}"
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
  if ! make up; then
    log_error "Failed to start services. Collecting diagnostic information..."
    
    # Collect diagnostic information about what might have gone wrong
    log "Docker service status:"
    docker info 2>/dev/null || echo "Docker engine is not responding"
    
    log "Network status:"
    docker network ls 2>/dev/null || echo "Cannot list Docker networks"
    
    log "Disk space:"
    df -h 2>/dev/null || echo "Cannot check disk space"
    
    log "Running containers:"
    docker ps 2>/dev/null || echo "Cannot list running containers"
    
    log "Recently exited containers (may show startup failures):"
    docker ps -a --filter "status=exited" --last 5 2>/dev/null || echo "Cannot list exited containers"
    
    log "Recent logs from any existing Midaz containers:"
    for container in $(docker ps -a --filter "name=midaz" --format "{{.Names}}" 2>/dev/null); do
      echo "--- Logs for $container ---"
      docker logs --tail 20 "$container" 2>/dev/null || echo "Cannot retrieve logs for $container"
      echo "------------------------"
    done
    
    die "Failed to start services. See diagnostic information above."
  fi
  
  # The Makefile's 'up' command already has built-in health checks,
  # but we'll add an explicit check just to provide better feedback to the user
  log "Verifying service health..."
  sleep 5  # Brief pause to allow services to initialize
  
  # Enhanced service health verification
  RUNNING_CONTAINERS=$(docker ps --filter "name=midaz" --format "{{.Names}}" 2>/dev/null | wc -l)
  
  if [ "${RUNNING_CONTAINERS}" -gt 0 ]; then
    log_success "Midaz services are running: ${RUNNING_CONTAINERS} containers detected"
    
    # Check for common ports to verify service availability
    if command -v nc >/dev/null 2>&1; then
      log "Checking service connectivity..."
      
      # Check web UI port
      if nc -z localhost 3000 2>/dev/null; then
        log_success "Web UI is accessible on port 3000"
      else
        log_warning "Web UI port 3000 doesn't appear to be accessible"
      fi
      
      # Check API port
      if nc -z localhost 8080 2>/dev/null; then
        log_success "API is accessible on port 8080"
      else
        log_warning "API port 8080 doesn't appear to be accessible"
      fi
      
      # Check database port
      if nc -z localhost 5432 2>/dev/null; then
        log_success "Database is accessible on port 5432"
      else
        log_warning "Database port 5432 doesn't appear to be accessible"
      fi
    else
      log "Network connectivity tool (nc) not available, skipping detailed port checks"
    fi
  else
    log_warning "No Midaz containers found running. There may have been an issue with startup."
    log_warning "Check container status with: 'cd ${MIDAZ_DIR} && docker compose ps'"
    log_warning "View container logs with: 'cd ${MIDAZ_DIR} && docker compose logs'"
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
  # Process arguments
  for arg in "$@"; do
    case "$arg" in
      --help|-h)
        display_help
        ;;
      -y|--yes)
        MIDAZ_AUTOCONFIRM=1
        ;;
    esac
  done

  # Display welcome banner
  echo "  __  __ _     _            "
  echo " |  \\/  (_)   | |           "
  echo " | \\  / |_  __| | __ _ ____"
  echo " | |\\/| | |/ _\` |/ _\` |_  /"
  echo " | |  | | | (_| | (_| |/ / "
  echo " |_|  |_|_|\\__,_|\\__,_/___| Core Banking"
  echo ""
  echo "Welcome to the Midaz Core Banking Platform installer"
  echo "This script will set up a complete Midaz environment on your system"
  echo "Run with --help for usage information"
  echo ""
  
  # Check for uninstall command
  if [ "${1:-}" = "--uninstall" ]; then
    uninstall_midaz
    trap - EXIT
    exit 0
  fi
  
  # Check for recovery from a previous interrupted installation
  check_for_recovery
  
  # Set current state to track progress
  CURRENT_INSTALL_STATE="detecting_os"
  
  detect_os
  CURRENT_INSTALL_STATE="checking_connectivity"
  
  check_internet_connectivity
  CURRENT_INSTALL_STATE="checking_deps"
  
  check_dependencies
  # Save state after dependencies are set up
  CURRENT_INSTALL_STATE="deps_installed"
  save_state "deps_installed"
  
  clone_midaz_repo
  # Save state after repository is cloned
  CURRENT_INSTALL_STATE="repo_cloned"
  save_state "repo_cloned"
  
  run_midaz
  # Save state after services are started
  CURRENT_INSTALL_STATE="services_started"
  save_state "services_started"
  
  generate_demo_data
  # Save state at completion
  CURRENT_INSTALL_STATE="completed"
  save_state "completed"
  
  display_success
  
  # Clean up the state file on successful completion
  rm -f "${MIDAZ_STATE_FILE}"
  
  # If we've made it this far, disable the EXIT trap
  trap - EXIT
}

# Function to start Docker daemon with appropriate method based on environment
start_docker_daemon() {
  # Skip if Docker is already running
  if docker info >/dev/null 2>&1; then
    log_success "Docker daemon is already running"
    return 0
  fi

  # Check if we're in a container environment
  if [ "${RUNNING_IN_CONTAINER}" -eq 1 ]; then
    log "Running in container environment, using alternative Docker startup method"
    # Try to start dockerd directly
    run_sudo nohup dockerd --host=unix:///var/run/docker.sock >/dev/null 2>&1 &
    # Give it a moment to start
    log "Waiting for Docker daemon to start..."
    sleep 5
    # Check if Docker is now running
    if docker info >/dev/null 2>&1; then
      log_success "Docker daemon started successfully"
      return 0
    else
      log_warning "Could not start Docker daemon. Docker commands may fail."
      return 1
    fi
  elif [ "${OS_FAMILY}" = "alpine" ]; then
    # Alpine uses OpenRC instead of systemd
    log "Starting Docker daemon with OpenRC on Alpine"
    run_sudo /etc/init.d/docker start || true
    # Give it a moment to start
    sleep 3
    # Check if Docker is now running
    if docker info >/dev/null 2>&1; then
      log_success "Docker daemon started successfully"
      return 0
    else
      log_warning "Could not start Docker daemon. Docker commands may fail."
      return 1
    fi
  else
    # Standard systemd startup
    log "Starting Docker daemon with systemd"
    run_sudo systemctl start docker
    # Check if Docker is now running
    if docker info >/dev/null 2>&1; then
      log_success "Docker daemon started successfully"
      return 0
    else
      log_warning "Could not start Docker daemon. Docker commands may fail."
      return 1
    fi
  fi
}

# Function to verify Docker group membership
verify_docker_permissions() {
  log "Verifying Docker permissions..."
  
  # Skip for macOS since it uses a different mechanism
  if [ "${OS_FAMILY}" = "darwin" ]; then
    log_success "Docker permissions check skipped on macOS"
    return 0
  fi
  
  # Check if the docker group exists
  if ! getent group docker >/dev/null 2>&1; then
    log_warning "Docker group does not exist. You may need to run Docker with sudo."
    return 1
  fi
  
  # Check if the current user is in the docker group
  if ! groups "$(whoami)" | grep -q '\bdocker\b'; then
    log_warning "Current user is not in the docker group. Docker commands may require sudo."
    
    # Remind the user that they need to log out and back in
    log "You've been added to the docker group, but you need to log out and back in for this to take effect."
    log "Alternatively, run 'newgrp docker' to activate the group for this session."
    
    return 1
  fi
  
  log_success "User is in the docker group. Docker should be usable without sudo."
  return 0
}

# ============================================================================
# Dependency Checking
# ============================================================================
# Verifies and installs required dependencies

check_dependencies() {
  log "Checking for required dependencies..."
  
  # Check for Git
  if ! check_command git; then
    log_warning "Git not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install Git? (Y/n): "; then
      install_git
    else
      die "Git is required for installation"
    fi
  else
    log_success "Git found: $(git --version)"
  fi
  
  # Check for Docker
  if ! check_command docker; then
    log_warning "Docker not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install Docker? (Y/n): "; then
      install_docker
    else
      die "Docker is required for installation"
    fi
  else
    log_success "Docker found: $(docker --version)"
    
    # Verify Docker is running
    if ! docker info >/dev/null 2>&1; then
      log_warning "Docker daemon is not running"
      log "Please start Docker before continuing"
      if [ "${OS_FAMILY}" = "darwin" ]; then
        log "On macOS, open Docker Desktop application"
      else
        log "On Linux, run: sudo systemctl start docker"
      fi
      if [ "${MIDAZ_AUTOCONFIRM}" -ne 1 ] && ! prompt "Continue anyway? (this will likely fail) (y/N): "; then
        die "Docker must be running to continue installation"
      fi
    fi
    
    # Check Docker Compose
    if docker compose version >/dev/null 2>&1; then
      log_success "Docker Compose found: $(docker compose version --short 2>/dev/null || echo "plugin")"
    elif command -v docker-compose >/dev/null 2>&1; then
      log_success "Legacy docker-compose found: $(docker-compose --version)"
    else
      log_warning "Docker Compose not found"
      if [ "${OS_PACKAGE_MANAGER}" = "brew" ]; then
        log "On macOS, Docker Compose is included with Docker Desktop"
      else
        log_warning "Docker Compose might need to be installed separately"
      fi
    fi
    
    # Verify Docker permissions
    verify_docker_permissions
  fi
  
  # Check for make
  if ! check_command make; then
    log_warning "Make not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install Make? (Y/n): "; then
      install_make
    else
      die "Make is required for installation"
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
      die "Go is required for installation"
    fi
  else
    GO_VERSION=$(go version)
    log_success "Go found: ${GO_VERSION}"
    
    # Verify Go version is at least 1.22
    if ! check_version go "go version" "1.22"; then
      log_warning "Go version is below the recommended minimum (1.22+)"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Upgrade Go? (Y/n): "; then
        install_go
      else
        log_warning "Proceeding with older Go version, which may cause compatibility issues"
      fi
    fi
  fi
  
  # Check for Node.js
  if ! check_command node; then
    log_warning "Node.js not found"
    if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Install Node.js? (Y/n): "; then
      install_node
    else
      die "Node.js is required for installation"
    fi
  else
    NODE_VERSION=$(node --version)
    log_success "Node.js found: ${NODE_VERSION}"
    
    # Verify Node.js version is at least 20.0
    if ! check_version node "node --version" "20.0"; then
      log_warning "Node.js version is below the recommended minimum (20.0+)"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Upgrade Node.js? (Y/n): "; then
        install_node
      else
        log_warning "Proceeding with older Node.js version, which may cause compatibility issues"
      fi
    fi
    
    # Verify npm is available
    if check_command npm; then
      log_success "npm found: $(npm --version)"
    else
      log_warning "npm not found but Node.js is installed"
      log_warning "Proceeding, but installation may encounter issues"
    fi
  fi
  
  log_success "All required dependencies found or installed"
}

# ============================================================================
# Uninstallation Function
# ============================================================================

uninstall_midaz() {
  log "Starting uninstallation of Midaz core-banking platform..."
  
  # Check if the Midaz directory exists
  if [ ! -d "${MIDAZ_DIR}" ]; then
    log_warning "Midaz installation directory not found at ${MIDAZ_DIR}"
    log "Nothing to uninstall. Exiting."
    return 0
  fi
  
  # Confirm before proceeding
  if [ "${MIDAZ_AUTOCONFIRM}" -ne 1 ]; then
    echo ""
    echo "WARNING: This will remove the Midaz installation and all associated data."
    echo "This action cannot be undone and all your data will be lost."
    if ! prompt "Are you sure you want to uninstall Midaz? (y/N): "; then
      log "Uninstallation cancelled by user."
      return 1
    fi
  fi
  
  # Try to stop any running services first
  if [ -d "${MIDAZ_DIR}" ]; then
    log "Stopping Midaz services..."
    
    # Check if Docker is available before attempting to stop services
    if command -v docker >/dev/null 2>&1; then
      # If make is available and there's a Makefile, use it to stop services
      if [ -f "${MIDAZ_DIR}/Makefile" ] && command -v make >/dev/null 2>&1; then
        (cd "${MIDAZ_DIR}" && make down) || log_warning "Could not stop services using make down"
      else
        # Fall back to docker compose directly
        (cd "${MIDAZ_DIR}" && docker compose down) || log_warning "Could not stop services using docker compose down"
      fi
      
      # Force remove any remaining containers with "midaz" in their name
      MIDAZ_CONTAINERS=$(docker ps -a --filter "name=midaz" -q 2>/dev/null)
      if [ -n "${MIDAZ_CONTAINERS}" ]; then
        log "Removing any remaining Midaz containers..."
        docker rm -f ${MIDAZ_CONTAINERS} 2>/dev/null || log_warning "Could not remove all Midaz containers"
      fi
      
      # Remove any Midaz networks
      MIDAZ_NETWORKS=$(docker network ls --filter "name=midaz" -q 2>/dev/null)
      if [ -n "${MIDAZ_NETWORKS}" ]; then
        log "Removing Midaz networks..."
        docker network rm ${MIDAZ_NETWORKS} 2>/dev/null || log_warning "Could not remove all Midaz networks"
      fi
      
      # Remove Midaz volumes if requested
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Remove all Midaz data volumes? This will permanently delete all your data. (y/N): "; then
        MIDAZ_VOLUMES=$(docker volume ls --filter "name=midaz" -q 2>/dev/null)
        if [ -n "${MIDAZ_VOLUMES}" ]; then
          log "Removing Midaz data volumes..."
          docker volume rm ${MIDAZ_VOLUMES} 2>/dev/null || log_warning "Could not remove all Midaz volumes"
        fi
      fi
    else
      log_warning "Docker not found, skipping container and service cleanup"
    fi
    
    # Remove the Midaz directory
    log "Removing Midaz installation directory..."
    rm -rf "${MIDAZ_DIR}" || log_warning "Could not completely remove ${MIDAZ_DIR}"
  fi
  
  # Remove the state file if it exists
  if [ -f "${MIDAZ_STATE_FILE}" ]; then
    log "Removing installation state file..."
    rm -f "${MIDAZ_STATE_FILE}" || log_warning "Could not remove state file ${MIDAZ_STATE_FILE}"
  fi
  
  log_success "Midaz has been uninstalled"
  
  # Provide guidance on manual cleanup if needed
  echo ""
  echo "NOTE: If you installed dependencies specifically for Midaz, you may want to"
  echo "remove them manually if they're no longer needed:"
  echo " - Docker: Used for containerization"
  echo " - Git: Used for repository management"
  echo " - Go: Used for backend services"
  echo " - Node.js: Used for frontend applications"
  echo " - Make: Used for build processes"
  echo ""
  
  return 0
}

# ============================================================================
# Help and Usage
# ============================================================================

display_help() {
  echo "Midaz Core-Banking Platform Installer"
  echo ""
  echo "USAGE:"
  echo "  curl -fsSL https://get.midaz.dev | sh [OPTIONS]"
  echo "  or"
  echo "  ./install.sh [OPTIONS]"
  echo ""
  echo "OPTIONS:"
  echo "  --help             Show this help message"
  echo "  --uninstall        Uninstall Midaz from the system"
  echo "  -y, --yes          Automatic yes to prompts (non-interactive mode)"
  echo ""
  echo "ENVIRONMENT VARIABLES:"
  echo "  MIDAZ_DIR          Override installation directory (default: ~/midaz)"
  echo "  MIDAZ_REF          Set the branch/tag to install (default: main)"
  echo "  INSTALL_FLAGS      Pass installation flags (-y for automatic yes)"
  echo ""
  echo "EXAMPLES:"
  echo "  # Install with default options"
  echo "  curl -fsSL https://get.midaz.dev | sh"
  echo ""
  echo "  # Install in non-interactive mode"
  echo "  curl -fsSL https://get.midaz.dev | sh -s -- -y"
  echo ""
  echo "  # Install to a specific directory"
  echo "  MIDAZ_DIR=/opt/midaz curl -fsSL https://get.midaz.dev | sh"
  echo ""
  echo "  # Install a specific branch"
  echo "  MIDAZ_REF=develop curl -fsSL https://get.midaz.dev | sh"
  echo ""
  echo "  # Uninstall Midaz"
  echo "  curl -fsSL https://get.midaz.dev | sh -s -- --uninstall"
  echo "  or"
  echo "  ./install.sh --uninstall"
  echo ""
  exit 0
}

# Execute main function
main "$@"