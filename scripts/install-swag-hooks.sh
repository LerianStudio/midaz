#!/usr/bin/env bash
# shellcheck disable=SC2155  # Declare and assign separately to avoid masking return values
set -euo pipefail

# Swag Pre-commit Hook Installer
# Installs an optional pre-commit hook that validates swag comments before commits.
#
# Usage:
#   ./scripts/install-swag-hooks.sh [--uninstall]
#
# Options:
#   --uninstall    Remove the swag pre-commit hook
#
# This script adds swag comment validation to the existing pre-commit hook
# or creates a new one if none exists.
#
# Note on bypassing:
#   Developers can bypass this hook using: git commit --no-verify
#   Hook bypasses are NOT logged by default. For production environments
#   requiring compliance audit trails, consider implementing bypass logging
#   at the CI/CD or repository level.

# Root directory of the repo
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Git hooks directory (relative to the midaz app, looking for monorepo root)
MONOREPO_ROOT="$(cd "${ROOT_DIR}/../.." && pwd)"
HOOKS_DIR="${MONOREPO_ROOT}/.git/hooks"
HOOK_FILE="${HOOKS_DIR}/pre-commit"

# Marker for our hook section
HOOK_MARKER_START="# === MIDAZ SWAG LINTING START ==="
HOOK_MARKER_END="# === MIDAZ SWAG LINTING END ==="

# Print status
print_status() {
    local status="$1"
    local message="$2"

    if [ "$status" = "OK" ]; then
        echo -e "${GREEN}[OK]${NC} $message"
    elif [ "$status" = "WARN" ]; then
        echo -e "${YELLOW}[WARN]${NC} $message"
    elif [ "$status" = "ERROR" ]; then
        echo -e "${RED}[ERROR]${NC} $message"
    else
        echo -e "[INFO] $message"
    fi
}

# Check if we're in a git repository
check_git_repo() {
    if [ ! -d "${MONOREPO_ROOT}/.git" ]; then
        print_status "ERROR" "Not in a git repository. Please run from within the monorepo."
        exit 1
    fi
}

# Create hooks directory if needed
ensure_hooks_dir() {
    if [ ! -d "$HOOKS_DIR" ]; then
        mkdir -p "$HOOKS_DIR"
        print_status "OK" "Created hooks directory"
    fi
}

# Generate the swag hook content
generate_swag_hook_content() {
    cat << 'HOOK_EOF'
# === MIDAZ SWAG LINTING START ===
# Check for changes in HTTP handler files
MIDAZ_DIR="apps/midaz"
HANDLER_PATTERN="components/*/internal/adapters/http/in/*.go"

# Check if any handler files are being committed
STAGED_HANDLERS=$(git diff --cached --name-only | grep -E "^${MIDAZ_DIR}/${HANDLER_PATTERN}" || true)

if [ -n "$STAGED_HANDLERS" ]; then
    echo "Running swag comment linter on staged handler files..."

    # Run the swag linter
    if [ -f "${MIDAZ_DIR}/scripts/lint-swag-comments.sh" ]; then
        if ! (cd "${MIDAZ_DIR}" && ./scripts/lint-swag-comments.sh); then
            echo ""
            echo "[ERROR] Swag linting failed. Please fix the issues before committing."
            echo "You can run 'make lint-swag' in apps/midaz for more details."
            echo "To bypass this check (not recommended), use: git commit --no-verify"
            echo ""
            echo "Note: Hook bypasses are not logged. For compliance tracking, consider"
            echo "implementing audit logging if required by your organization's policies."
            exit 1
        fi
    else
        echo "[WARN] Swag linter script not found, skipping check"
    fi
fi
# === MIDAZ SWAG LINTING END ===
HOOK_EOF
}

# Check if hook already has our section
hook_has_swag_section() {
    if [ -f "$HOOK_FILE" ]; then
        grep -q "$HOOK_MARKER_START" "$HOOK_FILE"
        return $?
    fi
    return 1
}

# Remove swag section from hook
remove_swag_section() {
    if [ -f "$HOOK_FILE" ] && hook_has_swag_section; then
        # Remove the section between markers (inclusive)
        sed -i.bak "/${HOOK_MARKER_START}/,/${HOOK_MARKER_END}/d" "$HOOK_FILE"
        rm -f "${HOOK_FILE}.bak"
        print_status "OK" "Removed swag linting section from pre-commit hook"
    else
        print_status "WARN" "No swag linting section found in pre-commit hook"
    fi
}

# Install the hook
install_hook() {
    check_git_repo
    ensure_hooks_dir

    # Check if hook already has our section
    if hook_has_swag_section; then
        print_status "WARN" "Swag linting hook already installed"
        echo "Use --uninstall to remove it first if you want to reinstall."
        return 0
    fi

    # If hook doesn't exist, create it
    if [ ! -f "$HOOK_FILE" ]; then
        cat > "$HOOK_FILE" << 'BASIC_HOOK'
#!/bin/bash
# Pre-commit hook with swag linting

set -e

BASIC_HOOK
        chmod +x "$HOOK_FILE"
        print_status "OK" "Created new pre-commit hook"
    fi

    # Make sure hook is executable
    if [ ! -x "$HOOK_FILE" ]; then
        chmod +x "$HOOK_FILE"
    fi

    # Append our hook content
    echo "" >> "$HOOK_FILE"
    generate_swag_hook_content >> "$HOOK_FILE"

    print_status "OK" "Installed swag linting pre-commit hook"
    echo ""
    echo "The pre-commit hook will now check swag comments when you commit"
    echo "changes to HTTP handler files in apps/midaz."
    echo ""
    echo "To uninstall: ./scripts/install-swag-hooks.sh --uninstall"
    echo "To bypass temporarily: git commit --no-verify"
}

# Main
main() {
    echo "Midaz Swag Pre-commit Hook Installer"
    echo "====================================="
    echo ""

    case "${1:-}" in
        --uninstall)
            remove_swag_section
            ;;
        --help)
            echo "Usage: $0 [--uninstall]"
            echo ""
            echo "Options:"
            echo "  --uninstall    Remove the swag pre-commit hook"
            echo "  --help         Show this help message"
            ;;
        *)
            install_hook
            ;;
    esac
}

main "$@"
