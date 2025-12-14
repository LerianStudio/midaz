#!/usr/bin/env bash
# Ensures LCRYPTO_ENCRYPT_SECRET_KEY in .env files is a valid 64-character hex string
# Usage: ./ensure-crypto-keys.sh [component_dir]
#   If component_dir is provided, only checks that component
#   If omitted, checks all components

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to check if a string is a valid 64-character hex string
is_valid_hex_key() {
    local key="$1"
    # Must be exactly 64 characters and only contain hex digits (0-9, a-f, A-F)
    if [[ ${#key} -eq 64 ]] && [[ "$key" =~ ^[0-9a-fA-F]{64}$ ]]; then
        return 0
    else
        return 1
    fi
}

# Function to process a single .env file
process_env_file() {
    local env_file="$1"
    local component_name=$(basename $(dirname "$env_file"))

    if [ ! -f "$env_file" ]; then
        return 0
    fi

    # Check if the file has LCRYPTO_ENCRYPT_SECRET_KEY
    if ! grep -q "^LCRYPTO_ENCRYPT_SECRET_KEY=" "$env_file"; then
        return 0  # No crypto key needed in this component
    fi

    # Extract the current key value
    local current_key=$(grep "^LCRYPTO_ENCRYPT_SECRET_KEY=" "$env_file" | cut -d'=' -f2- | tr -d '"' | tr -d "'")

    # Check if the key is valid
    if is_valid_hex_key "$current_key"; then
        echo -e "${GREEN}✓${NC} $component_name: Crypto key is valid"
        return 0
    fi

    # Key is invalid, generate a new one
    echo -e "${YELLOW}⚠${NC}  $component_name: Invalid crypto key detected (${#current_key} chars, expected 64 hex chars)"
    echo "  Generating new AES-256 key..."

    local new_key=$(openssl rand -hex 32)

    # Replace the key in the file
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS sed requires empty string for -i
        sed -i '' "s|^LCRYPTO_ENCRYPT_SECRET_KEY=.*|LCRYPTO_ENCRYPT_SECRET_KEY=$new_key|" "$env_file"
    else
        # GNU sed
        sed -i "s|^LCRYPTO_ENCRYPT_SECRET_KEY=.*|LCRYPTO_ENCRYPT_SECRET_KEY=$new_key|" "$env_file"
    fi

    echo -e "${GREEN}✓${NC} $component_name: Crypto key generated and updated"
}

# Main script
main() {
    local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local project_root="$(cd "$script_dir/.." && pwd)"

    if [ $# -eq 0 ]; then
        # No arguments - check all components
        echo "Checking crypto keys in all components..."

        for component_dir in "$project_root"/components/*/; do
            if [ -d "$component_dir" ]; then
                env_file="${component_dir}.env"
                process_env_file "$env_file"
            fi
        done
    else
        # Specific component provided
        local component_dir="$1"
        if [ ! -d "$component_dir" ]; then
            echo -e "${RED}Error:${NC} Directory not found: $component_dir" >&2
            exit 1
        fi

        env_file="${component_dir}/.env"
        process_env_file "$env_file"
    fi

    echo -e "${GREEN}✓${NC} Crypto key validation complete"
}

main "$@"
