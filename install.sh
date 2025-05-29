#!/usr/bin/env sh
# Midaz Core-Banking Platform Installer
# Automates setup of complete banking stack with Docker containers
# Supports Linux/macOS with interactive branch selection and recovery
# Strict error handling: exit on command failure, undefined vars, pipe failures
set -eu
set -o pipefail

# Safe field separator to handle whitespace in paths
IFS="$(printf '\n\t')"

# Check for existing Midaz containers
check_existing_installation() {
  log "Checking for existing Midaz containers..."
  
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    # Check for running Midaz containers
    RUNNING_MIDAZ=$(docker ps --filter "name=midaz" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$RUNNING_MIDAZ" ]; then
      echo ""
      echo "WARNING: Found running Midaz containers:"
      echo "$RUNNING_MIDAZ" | sed 's/^/  - /'
      echo ""
      echo "This installation may conflict with existing containers."
      echo "Options:"
      echo "  1. Stop existing containers first: 'docker stop \$(docker ps -q --filter name=midaz)'"
      echo "  2. Use different installation directory"
      echo "  3. Continue anyway (may cause conflicts)"
      echo ""
      
      if [ "${MIDAZ_AUTOCONFIRM}" -ne 1 ]; then
        if ! prompt "Continue with existing containers running? (y/N): "; then
          die "Installation aborted due to existing containers"
        fi
        log_warning "Proceeding with existing containers - conflicts may occur"
      else
        log_warning "Auto-confirming with existing containers (conflicts possible)"
      fi
    else
      log "No conflicting Midaz containers found"
    fi
  else
    log "Docker not available - skipping container check"
  fi
}

# Interactive installation directory selection
select_installation_directory() {
  if [ -n "${MIDAZ_DIR}" ]; then
    log "Using pre-configured directory: ${MIDAZ_DIR}"
    return 0
  fi
  
  if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ]; then
    # Non-interactive: use default
    MIDAZ_DIR="$HOME/midaz"
    log "Non-interactive mode: using ${MIDAZ_DIR}"
    return 0
  fi
  
  echo ""
  echo "=== INSTALLATION DIRECTORY ==="
  echo "Choose where to install Midaz:"
  echo ""
  echo "  1. $HOME/midaz (default)"
  echo "  2. /opt/midaz (system-wide, requires sudo)"
  echo "  3. ./midaz (current directory)"
  echo "  4. Custom path"
  echo ""
  printf "Select option [1-4]: "
  read -r dir_choice
  
  case "$dir_choice" in
    1|"")
      MIDAZ_DIR="$HOME/midaz"
      ;;
    2)
      MIDAZ_DIR="/opt/midaz"
      log_warning "System-wide installation will require sudo privileges"
      ;;
    3)
      MIDAZ_DIR="$(pwd)/midaz"
      ;;
    4)
      printf "Enter custom installation path: "
      read -r custom_path
      if [ -z "$custom_path" ]; then
        log_warning "Empty path provided, using default"
        MIDAZ_DIR="$HOME/midaz"
      else
        MIDAZ_DIR="$custom_path"
      fi
      ;;
    *)
      log_warning "Invalid choice, using default"
      MIDAZ_DIR="$HOME/midaz"
      ;;
  esac
  
  echo ""
  log "Selected installation directory: ${MIDAZ_DIR}"
  
  # Check if directory exists and has content
  if [ -d "${MIDAZ_DIR}" ] && [ "$(ls -A "${MIDAZ_DIR}" 2>/dev/null)" ]; then
    echo ""
    log_warning "Directory ${MIDAZ_DIR} exists and is not empty"
    if ! prompt "Continue and potentially overwrite contents? (y/N): "; then
      die "Installation aborted - directory not empty"
    fi
  fi
}

# Security validation for environment variables
validate_env_vars() {
  # Select installation directory if not set
  select_installation_directory
  
  # Check MIDAZ_DIR for shell injection patterns
  case "${MIDAZ_DIR:-}" in
    *";"*|*"|"*|*"&"*|*"$"*|*"`"*|*"\\"*)
      die "MIDAZ_DIR contains dangerous characters"
      ;;
    "")
      die "MIDAZ_DIR cannot be empty"
      ;;
    /*)
      # Absolute path - validate length
      if [ "${#MIDAZ_DIR}" -gt 200 ]; then
        die "MIDAZ_DIR path too long (>200 chars)"
      fi
      ;;
    ~/*)
      # Home-relative path OK
      ;;
    *)
      # Convert relative to absolute
      MIDAZ_DIR="$(pwd)/${MIDAZ_DIR}"
      log_warning "Converting relative MIDAZ_DIR to absolute: ${MIDAZ_DIR}"
      ;;
  esac
  
  # Check MIDAZ_REF for shell injection
  case "${MIDAZ_REF:-}" in
    *";"*|*"|"*|*"&"*|*"$"*|*"`"*|*"\\"*|*".."*)
      die "MIDAZ_REF contains dangerous characters"
      ;;
  esac
  
  # Validate git ref format
  if [ -n "${MIDAZ_REF:-}" ]; then
    case "${MIDAZ_REF}" in
      -*)
        die "MIDAZ_REF cannot start with dash"
        ;;
      *" "*|*"\t"*|*"\n"*)
        die "MIDAZ_REF cannot contain whitespace"
        ;;
    esac
    
    if [ "${#MIDAZ_REF}" -gt 100 ]; then
      die "MIDAZ_REF too long (>100 chars)"
    fi
  fi
}

# Configuration variables (override via environment)
MIDAZ_DIR="${MIDAZ_DIR:-}"             # Install location (will prompt if empty)
MIDAZ_REF="${MIDAZ_REF:-main}"         # Git branch/tag
MIDAZ_REPO="https://github.com/lerianstudio/midaz"  # Repository URL
MIDAZ_AUTOCONFIRM=0                    # Interactive mode flag
INSTALL_FLAGS="${INSTALL_FLAGS:-}"     # Additional install flags
BRANCH_SELECTION_TIMEOUT=10            # Branch selection timeout

# State tracking for installation recovery
MIDAZ_STATE_FILE="/tmp/midaz_install_state.tmp"

# Save installation progress state
save_state() {
  state=$1
  echo "${state}" > "${MIDAZ_STATE_FILE}"
  log "State saved: ${state}"
}

# Read current installation state
read_state() {
  if [ -f "${MIDAZ_STATE_FILE}" ]; then
    cat "${MIDAZ_STATE_FILE}"
  else
    echo "not_started"
  fi
}

# Check for interrupted installation and offer recovery
check_for_recovery() {
  log "Checking for interrupted installation..."
  
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

# Signal cleanup handler - saves state and cleans temp files
cleanup() {
  exit_code=${1:-1}
  
  # Save current state for recovery
  if [ -n "${CURRENT_INSTALL_STATE:-}" ]; then
    save_state "${CURRENT_INSTALL_STATE}"
    echo "[MIDAZ] Installation state saved for recovery."
  fi
  
  # Clean up temporary files
  if [ -n "${TEMP_FILES:-}" ]; then
    log "Cleaning up temporary files..."
    for temp_file in $TEMP_FILES; do
      if [ -f "$temp_file" ]; then
        rm -f "$temp_file" 2>/dev/null || true
      fi
    done
  fi
  
  # Clean up artifact directories if installation failed
  if [ $exit_code -ne 0 ] && [ -n "${ARTIFACT_DIRS:-}" ]; then
    log "Cleaning up build artifacts..."
    for artifact_dir in $ARTIFACT_DIRS; do
      if [ -d "$artifact_dir" ]; then
        rm -rf "$artifact_dir" 2>/dev/null || true
      fi
    done
  fi
  
  echo "[MIDAZ] Installation interrupted. Exiting..."
  echo "[MIDAZ] You may need to manually clean up any partially installed components."
  echo "[MIDAZ] Run the installer again to attempt recovery."
  
  # Security error guidance
  if [ $exit_code -eq 2 ]; then
    echo "[MIDAZ] Security-related failure detected."
    echo "[MIDAZ] Review error messages and ensure you're using trusted sources."
  fi
  
  exit $exit_code
}

# Signal traps for graceful shutdown
trap 'cleanup 130' INT  # Ctrl+C
trap 'cleanup 143' TERM # Kill signal
trap 'cleanup' EXIT     # Script exit
trap 'cleanup 129' HUP  # Terminal disconnect

# Track temp files and artifacts for cleanup
TEMP_FILES=""
ARTIFACT_DIRS=""

# Add file to cleanup list
add_temp_file() {
  if [ -n "$1" ]; then
    TEMP_FILES="$TEMP_FILES $1"
  fi
}

# Add directory to artifact cleanup list
add_artifact_dir() {
  if [ -n "$1" ]; then
    ARTIFACT_DIRS="$ARTIFACT_DIRS $1"
  fi
}

# Logging functions with consistent formatting
log() {
  echo "[MIDAZ] $1"
}

log_success() {
  echo "[MIDAZ] SUCCESS: $1"
}

log_warning() {
  echo "[MIDAZ] WARNING: $1"
}

log_error() {
  echo "[MIDAZ] ERROR: $1" >&2
}

# Fatal error with diagnostic info and cleanup
die() {
  error_message="$1"
  exit_code="${2:-1}"
  
  log_error "$error_message"
  
  # Diagnostic information for troubleshooting
  echo "\nDiagnostic Information:"
  echo "===================================================================="
  echo "Error: $error_message"
  echo "Exit Code: $exit_code"
  echo "Timestamp: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  echo ""
  
  # System info
  if [ -f /etc/os-release ]; then
    echo "OS Information:"
    cat /etc/os-release
    echo ""
  fi
  echo "System: $(uname -a)"
  echo ""
  
  # Installation context
  echo "Installation Parameters:"
  echo "  Directory: ${MIDAZ_DIR}"
  echo "  Git reference: ${MIDAZ_REF}"
  echo "  Auto-confirm: ${MIDAZ_AUTOCONFIRM}"
  echo "  State: ${CURRENT_INSTALL_STATE:-unknown}"
  echo ""
  
  # Quick dependency checks
  echo "Dependencies:"
  echo "  Docker: $(command -v docker >/dev/null && echo 'Yes' || echo 'No')"
  echo "  Git: $(command -v git >/dev/null && echo 'Yes' || echo 'No')"
  echo "  Make: $(command -v make >/dev/null && echo 'Yes' || echo 'No')"
  echo "  Network: $(ping -c 1 8.8.8.8 >/dev/null 2>&1 && echo 'Yes' || echo 'No')"
  echo ""
  echo "Report issues to: support@midaz.dev"
  echo "===================================================================="
  
  cleanup $exit_code
}

# User confirmation prompt with safe defaults
prompt() {
  prompt_text="$1"
  
  # Auto-confirm in non-interactive mode
  if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ]; then
    log "Auto-confirming: $prompt_text"
    return 0
  fi
  
  printf "[MIDAZ] %s" "$prompt_text"
  
  # Use timeout to prevent hanging (5 min limit)
  if command -v timeout >/dev/null 2>&1; then
    response=$(timeout 300 sh -c 'read -r response && echo "$response"' || echo "timeout")
  else
    read -r response
  fi
  
  # Handle timeout
  if [ "$response" = "timeout" ]; then
    echo ""
    log_warning "No response within 5 minutes, assuming 'no'"
    return 1
  fi
  
  # Detect default behavior from prompt format
  # (Y/n) = default YES, (y/N) = default NO
  if echo "$prompt_text" | grep -q "(Y/n)"; then
    # Default YES prompts
    case "${response}" in
      [nN]|[nN][oO])
        return 1  # No
        ;;
      *)
        return 0  # Yes (including empty)
        ;;
    esac
  elif echo "$prompt_text" | grep -q "(y/N)"; then
    # Default NO prompts
    case "${response}" in
      [yY]|[yY][eE][sS])
        return 0  # Yes
        ;;
      *)
        return 1  # No (including empty)
        ;;
    esac
  else
    # Ambiguous prompts - require explicit yes
    case "${response}" in
      [yY]|[yY][eE][sS])
        return 0  # Yes
        ;;
      *)
        return 1  # No
        ;;
    esac
  fi
}

# Privilege escalation detection (sudo/doas)
get_sudo() {
  if [ "$(id -u)" = 0 ]; then
    echo ""  # Already root
  elif command -v sudo >/dev/null 2>&1; then
    echo "sudo"
  elif command -v doas >/dev/null 2>&1; then
    echo "doas"
  else
    return 1  # No privilege escalation available
  fi
}

# Run command with privilege escalation if needed
run_sudo() {
  # Try without sudo first for certain commands
  if [ -z "${FORCE_SUDO:-}" ]; then
    case "$1" in
      "apt-get")
        if [ "$2" = "update" ] && [ -w /var/lib/apt/lists ]; then
          log "Running apt-get update without sudo"
          "$@"
          return $?
        fi
        ;;
      "systemctl")
        if systemctl --user daemon-reload >/dev/null 2>&1; then
          log "Using systemctl user mode"
          systemctl --user "$2" "$3"
          return $?
        fi
        ;;
      "docker")
        if [ -S /var/run/docker.sock ] && [ -w /var/run/docker.sock ]; then
          log "Docker available without sudo"
          "$@"
          return $?
        elif groups "$(whoami)" | grep -q '\bdocker\b'; then
          log "Using docker group membership"
          "$@"
          return $?
        fi
        ;;
    esac
  fi
  
  # Use sudo/doas if available
  SUDO="$(get_sudo)"
  if [ -n "$SUDO" ]; then
    log "Running with elevated privileges: $*"
    
    if [ -z "${SUDO_PASSWORD_ENTERED:-}" ]; then
      log "You may be prompted for your password"
      SUDO_PASSWORD_ENTERED=1
    fi
    
    if ! $SUDO "$@"; then
      ERROR_CODE=$?
      if [ $ERROR_CODE -eq 1 ] || [ $ERROR_CODE -eq 126 ] || [ $ERROR_CODE -eq 127 ]; then
        log_warning "Command failed - check sudo privileges with 'sudo -v'"
      fi
      return $ERROR_CODE
    fi
  else
    log_warning "No sudo/doas available, running directly"
    "$@"
  fi
}

# Dependency checking functions

# Check if command exists in PATH
check_command() {
  command -v "$1" >/dev/null 2>&1
}

# Compare version numbers against minimum requirements
check_version() {
  command=$1
  version_command=$2
  min_version=$3
  
  # macOS version check handled separately
  if [ "$command" = "sw_vers" ]; then
    return 0
  fi

  # Safe command execution (no eval)
  case "$version_command" in
    "git --version")
      version_output=$(git --version 2>/dev/null)
      ;;
    "docker --version")
      version_output=$(docker --version 2>/dev/null)
      ;;
    "make --version")
      version_output=$(make --version 2>/dev/null)
      ;;
    "go version")
      version_output=$(go version 2>/dev/null)
      ;;
    "node --version")
      version_output=$(node --version 2>/dev/null)
      ;;
    "npm --version")
      version_output=$(npm --version 2>/dev/null)
      ;;
    *)
      log_warning "Unsupported version command: $version_command"
      return 1
      ;;
  esac
  
  # Extract version with fallback methods
  version=$(echo "$version_output" | grep -oE '[0-9]+(\.[0-9]+)+' | head -1)
  
  if [ -z "$version" ]; then
    version=$(echo "$version_output" | grep -oE '[0-9]+\.[0-9]+' | head -1)
    if [ -z "$version" ]; then
      version=$(echo "$version_output" | grep -oE '[0-9]+' | head -1)
      if [ -z "$version" ]; then
        log_warning "Could not determine version for $command"
        return 1
      fi
      version="${version}.0"  # Add .0 for single numbers
    fi
  fi
  
  # Parse major.minor versions
  major=$(echo "$version" | cut -d. -f1)
  minor=$(echo "$version" | cut -d. -f2)
  req_major=$(echo "$min_version" | cut -d. -f1)
  req_minor=$(echo "$min_version" | cut -d. -f2)
  
  # Version comparison
  if [ "$major" -gt "$req_major" ] || ([ "$major" -eq "$req_major" ] && [ "$minor" -ge "$req_minor" ]); then
    return 0  # Version OK
  else
    return 1  # Version too old
  fi
}

# Verify successful installation
verify_installation() {
  command=$1
  version_command=$2
  
  if ! check_command "$command"; then
    log_warning "$command not found in PATH after installation"
    return 1
  fi
  
  # Safe version check
  case "$version_command" in
    "git --version")
      version_output=$(git --version 2>&1)
      ;;
    "docker --version")
      version_output=$(docker --version 2>&1)
      ;;
    "make --version")
      version_output=$(make --version 2>&1)
      ;;
    *)
      log_warning "Unsupported version command: $version_command"
      return 1
      ;;
  esac
  
  if [ $? -ne 0 ]; then
    log_warning "Installed $command but version check failed: $version_output"
    return 1
  fi
  
  log_success "$command installed: $version_output"
  return 0
}

# Network utilities

# Get secure download tool (curl/wget with TLS 1.2+)
get_downloader() {
  if command -v curl >/dev/null 2>&1; then
    echo "curl -fsSL --tlsv1.2"  # Secure curl with TLS 1.2+
  elif command -v wget >/dev/null 2>&1; then
    echo "wget -q -O- --secure-protocol=TLSv1_2"  # Secure wget
  else
    die "Neither curl nor wget found"
  fi
}

# Safe download with retry and integrity checks
safe_download() {
  url="$1"
  output_file="$2"
  max_retries="${3:-3}"
  
  # Validate URL (HTTPS only)
  case "$url" in
    https://*)
      ;; # OK
    http://*)
      die "HTTP not allowed: $url"
      ;;
    *)
      die "Invalid URL: $url"
      ;;
  esac
  
  DOWNLOADER=$(get_downloader)
  
  retry_count=0
  while [ $retry_count -lt $max_retries ]; do
    log "Downloading (attempt $((retry_count + 1))/$max_retries): $url"
    
    if [ -n "$output_file" ]; then
      # Download to file with temp handling
      temp_file="$output_file.tmp"
      add_temp_file "$temp_file"  # Track for cleanup
      
      if $DOWNLOADER "$url" > "$temp_file"; then
        # Basic integrity check (non-empty, reasonable size)
        if [ -s "$temp_file" ] && [ "$(wc -c < "$temp_file")" -gt 10 ]; then
          mv "$temp_file" "$output_file"
          log_success "Downloaded: $output_file"
          return 0
        else
          log_warning "File empty or too small, retrying"
          rm -f "$temp_file"
        fi
      fi
    else
      # Download to stdout
      if $DOWNLOADER "$url"; then
        return 0
      fi
    fi
    
    retry_count=$((retry_count + 1))
    if [ $retry_count -lt $max_retries ]; then
      log "Retrying in 2 seconds..."
      sleep 2
    fi
  done
  
  die "Download failed after $max_retries attempts: $url"
}

# OS and architecture detection
detect_os() {
  log "Detecting OS and architecture..."
  
  # Normalize architecture names
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
      log_warning "Architecture ${ARCH} may not be fully supported"
      ;;
  esac
  
  log "Architecture: ${ARCH}"
  
  # Container detection
  if [ -f /.dockerenv ] || grep -q docker /proc/1/cgroup 2>/dev/null || grep -q container /proc/1/cgroup 2>/dev/null; then
    RUNNING_IN_CONTAINER=1
    log "Container environment detected"
  fi
  
  # Detect Linux distribution or macOS
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_NAME="${ID}"
    OS_VERSION="${VERSION_ID:-}"
    OS_FAMILY="linux"
    
    # Set package manager based on distribution
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
        log_warning "Unknown Linux distribution: ${OS_NAME}"
        OS_PACKAGE_MANAGER="unknown"
        ;;
    esac
  elif [ "$(uname)" = "Darwin" ]; then
    OS_NAME="macos"
    OS_FAMILY="darwin"
    OS_PACKAGE_MANAGER="brew"
    OS_VERSION="$(sw_vers -productVersion)"
  else
    die "Unsupported OS. Supports: Debian/Ubuntu, RHEL/CentOS/Fedora, Arch, macOS"
  fi
  
  log "OS: ${OS_NAME} ${OS_VERSION} (${OS_FAMILY}) on ${ARCH}"
  
  # Basic version compatibility warnings
  if [ "${OS_FAMILY}" = "darwin" ]; then
    # macOS version validation
    if ! echo "${OS_VERSION}" | grep -q "\."; then
      log_warning "Non-standard macOS version: ${OS_VERSION}"
      DETAILED_VERSION=$(sw_vers -productVersion 2>/dev/null)
      if [ -n "${DETAILED_VERSION}" ] && echo "${DETAILED_VERSION}" | grep -q "\."; then
        OS_VERSION="${DETAILED_VERSION}"
      else
        log_warning "Cannot parse macOS version, assuming compatible"
        return 0
      fi
    fi
    
    MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
    if [ "${MAJOR_VERSION}" -ge 11 ]; then
      : # macOS 11+ OK
    elif [ "${MAJOR_VERSION}" -eq 10 ]; then
      MINOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f2)
      if [ "${MINOR_VERSION}" -lt 15 ]; then
        die "macOS ${OS_VERSION} not supported. Need 10.15+"
      fi
    fi
  elif [ "${OS_FAMILY}" = "linux" ]; then
    # Linux version warnings (non-blocking)
    case "${OS_NAME}" in
      ubuntu)
        if [ -n "${OS_VERSION}" ]; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 20 ]; then
            log_warning "Ubuntu ${OS_VERSION} is old. Recommend 20.04+"
          fi
        fi
        ;;
      debian)
        if [ -n "${OS_VERSION}" ] && [ "${OS_VERSION}" -lt 10 ]; then
          log_warning "Debian ${OS_VERSION} is old. Recommend 10+"
        fi
        ;;
      fedora)
        if [ -n "${OS_VERSION}" ] && [ "${OS_VERSION}" -lt 34 ]; then
          log_warning "Fedora ${OS_VERSION} is old. Recommend 34+"
        fi
        ;;
      centos|rhel|rocky|alma)
        if [ -n "${OS_VERSION}" ]; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 8 ]; then
            log_warning "${OS_NAME} ${OS_VERSION} is old. Recommend 8+"
          fi
        fi
        ;;
      alpine)
        if [ -n "${OS_VERSION}" ]; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 3 ] || ([ "${MAJOR_VERSION}" -eq 3 ] && [ "$(echo "${OS_VERSION}" | cut -d. -f2)" -lt 14 ]); then
            log_warning "Alpine ${OS_VERSION} is old. Recommend 3.14+"
          fi
        fi
        ;;
      opensuse*|suse*)
        if echo "${OS_VERSION}" | grep -q "\."; then
          MAJOR_VERSION=$(echo "${OS_VERSION}" | cut -d. -f1)
          if [ "${MAJOR_VERSION}" -lt 15 ]; then
            log_warning "openSUSE ${OS_VERSION} is old. Recommend 15+"
          fi
        fi
        ;;
      *)
        log_warning "Unknown distribution ${OS_NAME}. Compatibility unknown"
        ;;
    esac
  fi
}

# ============================================================================
# Docker Management Functions
# ============================================================================
#
# These functions handle Docker daemon management and permission verification
# across different operating systems and environments.
#
# The script checks if Docker is running and attempts to start it if needed,
# using the appropriate method for the detected operating system.

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

# Display Docker image info for manual security verification
verify_docker_images() {
  log "Docker image verification required"
  
  # Core images used by Midaz stack
  IMAGES="postgres:15-alpine rabbitmq:3-management mongo:6 redis:7-alpine"
  
  echo ""
  echo "=== DOCKER IMAGE VERIFICATION ==="
  echo "These Docker images will be downloaded:"
  echo ""
  
  for image in $IMAGES; do
    echo "Image: $image"
    
    # Show image info if Docker available
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
      # Local image ID if already pulled
      IMAGE_ID=$(docker images --format "{{.ID}}" "$image" 2>/dev/null | head -1)
      if [ -n "$IMAGE_ID" ]; then
        echo "  Local ID: $IMAGE_ID"
      fi
      
      # Remote digest for verification
      DIGEST=$(docker manifest inspect "$image" 2>/dev/null | grep '"digest"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "Unable to fetch")
      echo "  Digest: $DIGEST"
    else
      echo "  (Docker unavailable for inspection)"
    fi
    
    echo "  Verify: https://hub.docker.com/_/$image"
    echo ""
  done
  
  echo "SECURITY: Verify these digests match official Docker Hub images"
  echo "Optional: Use 'docker trust inspect <image>' for signature verification"
  echo ""
  
  if [ "${MIDAZ_AUTOCONFIRM}" -ne 1 ]; then
    if ! prompt "Have you verified these images are from trusted sources? (Y/n): "; then
      die "User declined image verification" 2
    fi
  else
    log "Auto-confirming image verification (non-interactive mode)"
  fi
}

# Fetch available git branches from GitHub API
get_available_branches() {
  log "Fetching branches from ${MIDAZ_REPO}..."
  
  # Use safe HTTPS download
  BRANCHES=$(safe_download "https://api.github.com/repos/lerianstudio/midaz/branches" | grep '"name"' | cut -d '"' -f 4)
  
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
  log "Installing Midaz repository to ${MIDAZ_DIR}..."
  log "Downloading core banking platform source code"
  
  # Track the directory for artifact cleanup
  add_artifact_dir "${MIDAZ_DIR}"
  
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
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Update existing repository? (Y/n): "; then
        log "Updating repository..."
        cd "${MIDAZ_DIR}"
        git fetch origin
        git checkout "${MIDAZ_REF}"
        git pull
      fi
    else
      log_warning "Directory ${MIDAZ_DIR} exists but is not a git repository"
      if [ "${MIDAZ_AUTOCONFIRM}" -eq 1 ] || prompt "Remove and clone fresh repository? (Y/n): "; then
        log "Removing existing directory and cloning..."
        rm -rf "${MIDAZ_DIR}"
        git clone --branch "${MIDAZ_REF}" --single-branch "${MIDAZ_REPO}" "${MIDAZ_DIR}"
      else
        die "Cannot proceed without clean repository"
      fi
    fi
  else
    log "Cloning Midaz repository (branch: ${MIDAZ_REF})..."
    # Create parent directory if needed
    parent_dir="$(dirname "${MIDAZ_DIR}")"
    if [ ! -d "$parent_dir" ]; then
      mkdir -p "$parent_dir" || die "Cannot create parent directory: $parent_dir"
    fi
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
  
  # Track artifact directories for potential cleanup
  add_artifact_dir "${MIDAZ_DIR}/components/infra/artifacts"
  add_artifact_dir "${MIDAZ_DIR}/components/onboarding/artifacts"
  add_artifact_dir "${MIDAZ_DIR}/components/transaction/artifacts"
  add_artifact_dir "${MIDAZ_DIR}/components/mdz/artifacts"
  
  # Check if environment needs setup
  if [ ! -f "${MIDAZ_DIR}/components/infra/.env" ] || 
     [ ! -f "${MIDAZ_DIR}/components/onboarding/.env" ] || 
     [ ! -f "${MIDAZ_DIR}/components/transaction/.env" ]; then
    log "Setting up environment configuration..."
    make set-env || die "Failed to set up environment files. Please check error messages above."
  fi
  
  # Verify Docker images before proceeding
  verify_docker_images
  
  # Ensure Docker daemon is running before proceeding
  log "Verifying Docker daemon status..."
  if ! docker info >/dev/null 2>&1; then
    log_warning "Docker daemon is not running. Attempting to start it..."
    start_docker_daemon || log_warning "Could not start Docker daemon automatically. Services may fail to start."
    
    # Verify Docker permissions after starting
    verify_docker_permissions
  else
    log_success "Docker daemon is running"
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
  # Display welcome banner first
  echo ""
  echo " ██      ███████ ██████  ██  █████  ███    ██"
  echo " ██      ██      ██   ██ ██ ██   ██ ████   ██"
  echo " ██      █████   ██████  ██ ███████ ██ ██  ██"
  echo " ██      ██      ██   ██ ██ ██   ██ ██  ██ ██"
  echo " ███████ ███████ ██   ██ ██ ██   ██ ██   ████"
  echo ""
  echo "                   midaz core banking"
  echo ""
  echo "Welcome to the Midaz Core Banking Platform installer"
  echo "This script will set up a complete Midaz environment on your system"
  echo "Run with --help for usage information"
  echo ""
  
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
  
  # Check for existing installation conflicts
  check_existing_installation
  
  # Validate environment variables and select directory
  validate_env_vars
  
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
  
  # Clean up state file and temp files on success
  rm -f "${MIDAZ_STATE_FILE}"
  
  # Clean up temp files (but keep artifacts on success)
  if [ -n "${TEMP_FILES:-}" ]; then
    for temp_file in $TEMP_FILES; do
      if [ -f "$temp_file" ]; then
        rm -f "$temp_file" 2>/dev/null || true
      fi
    done
  fi
  
  log "Installation completed successfully - artifacts preserved"
  
  # Disable EXIT trap for successful completion
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
  MISSING_DEPS=0
  
  # Check for Git
  if ! check_command git; then
    log_warning "Git not found. Please install Git before continuing."
    MISSING_DEPS=1
  else
    log_success "Git found: $(git --version)"
  fi
  
  # Check for Docker
  if ! check_command docker; then
    log_warning "Docker not found. Please install Docker before continuing."
    MISSING_DEPS=1
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
      log_warning "Docker Compose not found. Please install Docker Compose before continuing."
      MISSING_DEPS=1
    fi
    
    # Verify Docker permissions
    verify_docker_permissions
  fi
  
  # Check for make
  if ! check_command make; then
    log_warning "Make not found. Please install Make before continuing."
    MISSING_DEPS=1
  else
    log_success "Make found: $(make --version | head -1)"
  fi
  
  # If any dependencies are missing, exit with error
  if [ "${MISSING_DEPS}" -eq 1 ]; then
    die "One or more required dependencies are missing. Please install them and try again."
  fi
  
  log_success "All required dependencies found"
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