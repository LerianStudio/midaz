#!/bin/bash

# Script to sync Postman collection with OpenAPI documentation
# This script converts OpenAPI specs to Postman collections with improved examples and descriptions

# Exit on error
set -e

# Function to install Node.js
install_nodejs() {
    echo "Node.js is not installed. Attempting to install..."
    
    # Check the operating system
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        if command -v brew &> /dev/null; then
            echo "Installing Node.js via Homebrew..."
            brew install node
        else
            echo "Homebrew not found. Installing Homebrew first..."
            /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
            echo "Installing Node.js via Homebrew..."
            brew install node
        fi
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Linux
        if command -v apt-get &> /dev/null; then
            echo "Installing Node.js via apt..."
            curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
            sudo apt-get install -y nodejs
        elif command -v yum &> /dev/null; then
            echo "Installing Node.js via yum..."
            curl -fsSL https://rpm.nodesource.com/setup_18.x | sudo bash -
            sudo yum install -y nodejs
        else
            echo "Could not determine package manager. Please install Node.js manually."
            echo "Visit https://nodejs.org/ to download and install Node.js"
            exit 1
        fi
    else
        echo "Unsupported operating system. Please install Node.js manually."
        echo "Visit https://nodejs.org/ to download and install Node.js"
        exit 1
    fi
    
    # Verify installation
    if ! command -v node &> /dev/null; then
        echo "Failed to install Node.js. Please install it manually."
        echo "Visit https://nodejs.org/ to download and install Node.js"
        exit 1
    fi
    
    echo "Node.js installed successfully."
}

# Function to install jq
install_jq() {
    echo "jq is not installed. Attempting to install..."
    
    # Check the operating system
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        if command -v brew &> /dev/null; then
            echo "Installing jq via Homebrew..."
            brew install jq
        else
            echo "Homebrew not found. Installing Homebrew first..."
            /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
            echo "Installing jq via Homebrew..."
            brew install jq
        fi
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Linux
        if command -v apt-get &> /dev/null; then
            echo "Installing jq via apt..."
            sudo apt-get update
            sudo apt-get install -y jq
        elif command -v yum &> /dev/null; then
            echo "Installing jq via yum..."
            sudo yum install -y jq
        else
            echo "Could not determine package manager. Please install jq manually."
            echo "For installation instructions, visit: https://stedolan.github.io/jq/download/"
            exit 1
        fi
    else
        echo "Unsupported operating system. Please install jq manually."
        echo "For installation instructions, visit: https://stedolan.github.io/jq/download/"
        exit 1
    fi
    
    # Verify installation
    if ! command -v jq &> /dev/null; then
        echo "Failed to install jq. Please install it manually."
        echo "For installation instructions, visit: https://stedolan.github.io/jq/download/"
        exit 1
    fi
    
    echo "jq installed successfully."
}

# Check if Node.js is installed
if ! command -v node &> /dev/null; then
    install_nodejs
fi

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    install_jq
fi

# Define paths
MIDAZ_ROOT=$(pwd)
SCRIPTS_DIR="${MIDAZ_ROOT}/scripts"
CONVERTER="${SCRIPTS_DIR}/convert-openapi.js"  # Path to OpenAPI→Postman converter script
POSTMAN_DIR="${MIDAZ_ROOT}/postman"
TEMP_DIR="${MIDAZ_ROOT}/postman/temp"
ONBOARDING_API="${MIDAZ_ROOT}/components/onboarding/api"
TRANSACTION_API="${MIDAZ_ROOT}/components/transaction/api"
POSTMAN_COLLECTION="${POSTMAN_DIR}/MIDAZ.postman_collection.json"
POSTMAN_ENVIRONMENT="${POSTMAN_DIR}/MIDAZ.postman_environment.json"
BACKUP_DIR="${POSTMAN_DIR}/backups"

# Create necessary directories
mkdir -p "${TEMP_DIR}"
mkdir -p "${BACKUP_DIR}"

# Backup existing Postman collection and environment
TIMESTAMP=$(date +"%Y%m%d%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/MIDAZ.postman_collection.${TIMESTAMP}.json"
BACKUP_ENV_FILE="${BACKUP_DIR}/MIDAZ.postman_environment.${TIMESTAMP}.json"

if [ -f "${POSTMAN_COLLECTION}" ]; then
    echo "Backing up existing Postman collection to ${BACKUP_FILE}..."
    cp "${POSTMAN_COLLECTION}" "${BACKUP_FILE}"
fi

if [ -f "${POSTMAN_ENVIRONMENT}" ]; then
    echo "Backing up existing Postman environment to ${BACKUP_ENV_FILE}..."
    cp "${POSTMAN_ENVIRONMENT}" "${BACKUP_ENV_FILE}"
fi

# Install NPM dependencies if needed
if [ -f "${SCRIPTS_DIR}/package.json" ]; then
    echo "Installing NPM dependencies..."
    cd "${SCRIPTS_DIR}" && npm install
    cd "${MIDAZ_ROOT}"
fi

# Convert OpenAPI specs to Postman collections with environment templates
echo "Converting OpenAPI specs to Postman collections with improved examples..."

# Initialize failure flags
ONBOARDING_FAILED=false
TRANSACTION_FAILED=false
WORKFLOW_FAILED=false

# Process onboarding component
if [ -f "${ONBOARDING_API}/swagger.json" ]; then
    echo "Processing onboarding component..."
    node "${CONVERTER}" "${ONBOARDING_API}/swagger.json" "${TEMP_DIR}/onboarding.postman_collection.json" --env "${TEMP_DIR}/onboarding.environment.json"
    if [ $? -ne 0 ]; then
        echo "Failed to convert onboarding API spec to Postman collection."
        echo "Continuing with other components..."
        ONBOARDING_FAILED=true
    fi
else
    echo "Onboarding API spec not found. Skipping..."
fi

# Process transaction component
if [ -f "${TRANSACTION_API}/swagger.json" ]; then
    echo "Processing transaction component..."
    node "${CONVERTER}" "${TRANSACTION_API}/swagger.json" "${TEMP_DIR}/transaction.postman_collection.json" --env "${TEMP_DIR}/transaction.environment.json"
    if [ $? -ne 0 ]; then
        echo "Failed to convert transaction API spec to Postman collection."
        echo "Continuing with other components..."
        TRANSACTION_FAILED=true
    fi
else
    echo "Transaction API spec not found. Skipping..."
fi

# Merge collections and environment templates if needed
if [ -f "${TEMP_DIR}/onboarding.postman_collection.json" ] && [ -f "${TEMP_DIR}/transaction.postman_collection.json" ]; then
    echo "Merging collections..."
    
    # Create a new merged collection
    jq -s '
        # Merge items
        # Filter out any E2E Flow folders that might exist in individual components
        .[0].item = ([.[0].item[], .[1].item[]] | map(select(.name != "E2E Flow"))) | 
        
        # Combine variables from both collections
        if (.[0].variable != null and .[1].variable != null) then
            .[0].variable = (.[0].variable + .[1].variable | unique_by(.key))
        elif (.[1].variable != null) then
            .[0].variable = .[1].variable
        else
            .[0]
        end |
        
        # Update the collection name and ID
        .[0].info.name = "MIDAZ" | 
        .[0].info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560" |
        
        # Return the merged collection
        .[0]
    ' "${TEMP_DIR}/onboarding.postman_collection.json" "${TEMP_DIR}/transaction.postman_collection.json" > "${POSTMAN_COLLECTION}"
    
    # Merge environment templates
    if [ -f "${TEMP_DIR}/onboarding.environment.json" ] && [ -f "${TEMP_DIR}/transaction.environment.json" ]; then
        echo "Merging environment templates..."
        
        jq -s '
            # Use onboarding environment as base
            .[0].name = "MIDAZ Environment" |
            .[0].id = "midaz-environment-id" |
            
            # Combine values from both environments
            .[0].values = (.[0].values + .[1].values | unique_by(.key)) |
            
            # Return the merged environment
            .[0]
        ' "${TEMP_DIR}/onboarding.environment.json" "${TEMP_DIR}/transaction.environment.json" > "${POSTMAN_ENVIRONMENT}"
    elif [ -f "${TEMP_DIR}/onboarding.environment.json" ]; then
        echo "Using onboarding environment template..."
        jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' "${TEMP_DIR}/onboarding.environment.json" > "${POSTMAN_ENVIRONMENT}"
    elif [ -f "${TEMP_DIR}/transaction.environment.json" ]; then
        echo "Using transaction environment template..."
        jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' "${TEMP_DIR}/transaction.environment.json" > "${POSTMAN_ENVIRONMENT}"
    fi
    
elif [ -f "${TEMP_DIR}/onboarding.postman_collection.json" ]; then
    echo "Only onboarding component found. Using it as the main collection..."
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' "${TEMP_DIR}/onboarding.postman_collection.json" > "${POSTMAN_COLLECTION}"
    
    # Use onboarding environment if available
    if [ -f "${TEMP_DIR}/onboarding.environment.json" ]; then
        jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' "${TEMP_DIR}/onboarding.environment.json" > "${POSTMAN_ENVIRONMENT}"
    fi
    
elif [ -f "${TEMP_DIR}/transaction.postman_collection.json" ]; then
    echo "Only transaction component found. Using it as the main collection..."
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' "${TEMP_DIR}/transaction.postman_collection.json" > "${POSTMAN_COLLECTION}"
    
    # Use transaction environment if available
    if [ -f "${TEMP_DIR}/transaction.environment.json" ]; then
        jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' "${TEMP_DIR}/transaction.environment.json" > "${POSTMAN_ENVIRONMENT}"
    fi
    
else
    echo "No OpenAPI specs found. Make sure to generate the documentation first using 'make generate-docs-all'."
    exit 1
fi

# Clean up temporary files
echo "Cleaning up temporary files..."
rm -rf "${TEMP_DIR}"

# Add workflow sequence to the Postman collection
echo "Adding workflow sequence to Postman collection..."
if [ -f "${POSTMAN_COLLECTION}" ] && [ -f "${MIDAZ_ROOT}/postman/WORKFLOW.md" ]; then
    # Ensure uuid dependency is installed
    echo "Checking for required dependencies..."
    if ! grep -q "\"uuid\"" "${SCRIPTS_DIR}/package.json"; then
        echo "Adding uuid dependency to package.json..."
        # Use a temporary file to avoid issues with in-place editing
        jq '.dependencies.uuid = "^9.0.1"' "${SCRIPTS_DIR}/package.json" > "${SCRIPTS_DIR}/package.json.tmp"
        mv "${SCRIPTS_DIR}/package.json.tmp" "${SCRIPTS_DIR}/package.json"
        
        echo "Installing uuid dependency..."
        (cd "${SCRIPTS_DIR}" && npm install uuid)
    fi
    
    if node "${MIDAZ_ROOT}/scripts/create-workflow.js" "${POSTMAN_COLLECTION}" "${MIDAZ_ROOT}/postman/WORKFLOW.md" "${POSTMAN_COLLECTION}"; then
        echo "[ok] Workflow sequence added to Postman collection ✔️"
    else
        echo "[warning] Failed to add workflow sequence to Postman collection ⚠️"
        WORKFLOW_FAILED=true
    fi
else
    echo "[warning] Could not add workflow sequence: missing files ⚠️"
fi

echo "[ok] Postman collection and environment synced successfully with improved OpenAPI documentation ✔️"
echo "Note: The synced collection is available at ${POSTMAN_COLLECTION}"
echo "The environment template is available at ${POSTMAN_ENVIRONMENT}"
echo "Backups of previous files are available in ${BACKUP_DIR}"
echo ""

# Check if any critical operations failed
if [ "${ONBOARDING_FAILED}" = true ] && [ "${TRANSACTION_FAILED}" = true ]; then
    echo "[error] Both onboarding and transaction API conversions failed ❌"
    exit 1
fi

if [ "${WORKFLOW_FAILED}" = true ] && [ ! -f "${POSTMAN_COLLECTION}" ]; then
    echo "[error] Failed to create a valid Postman collection ❌"
    exit 1
fi

# Success exit code
exit 0