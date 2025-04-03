#!/bin/bash

# Script to run commands on the Go SDK


# Get the root directory
ROOT_DIR=$(pwd)
SDK_GO_DIR="${ROOT_DIR}/sdks/go"

# Check if the SDK directory exists
if [ ! -d "${SDK_GO_DIR}" ]; then
    echo "Error: Go SDK directory not found at ${SDK_GO_DIR}"
    exit 1
fi

# Check if a command was provided
if [ $# -eq 0 ]; then
    echo "Error: No command specified"
    echo -e "Usage: $0 <command>"
    echo -e "Available commands: build, test, lint, fmt, coverage, tidy, examples, list-examples, check-api-changes, update-api-hashes, env-setup, all"
    exit 1
fi

COMMAND=$1
TITLE=""
ACTION=""

# Map the command to a title and action
case "" in
    "build")
        TITLE="Building Go SDK"
        ACTION="build"
        ;;
    "test")
        TITLE="Running Go SDK tests"
        ACTION="test"
        ;;
    "lint")
        TITLE="Linting Go SDK"
        ACTION="lint"
        ;;
    "fmt")
        TITLE="Formatting Go SDK code"
        ACTION="fmt"
        ;;
    "coverage")
        TITLE="Generating Go SDK test coverage"
        ACTION="coverage"
        ;;
    "tidy")
        TITLE="Tidying Go SDK dependencies"
        ACTION="tidy"
        ;;
    "examples")
        TITLE="Building Go SDK examples"
        ACTION="examples"
        ;;
    "list-examples")
        TITLE="Listing available Go SDK examples"
        ACTION="list-examples"
        ;;
    "check-api-changes")
        TITLE="Checking Go SDK for API changes"
        ACTION="check-api-changes"
        ;;
    "update-api-hashes")
        TITLE="Updating Go SDK API hashes"
        ACTION="update-api-hashes"
        ;;
    "env-setup")
        TITLE="Setting up Go SDK environment"
        ACTION="env-setup"
        ;;
    "all")
        TITLE="Running all Go SDK commands"
        # For "all", we'll handle it specially
        ;;
    *)
        echo "Error: Unknown command ''"
        echo -e "Available commands: build, test, lint, fmt, coverage, tidy, examples, list-examples, check-api-changes, update-api-hashes, env-setup, all"
        exit 1
        ;;
esac

# Print the title
echo "------------------------------------------"
echo "   📝   "
echo "------------------------------------------"

# If the command is "all", run multiple commands
if [ "" = "all" ]; then
    echo "Running lint..."
    (cd "${SDK_GO_DIR}" && make lint)
    
    echo "Running fmt..."
    (cd "${SDK_GO_DIR}" && make fmt)
    
    echo "Running tidy..."
    (cd "${SDK_GO_DIR}" && make tidy)
    
    echo "Running build..."
    (cd "${SDK_GO_DIR}" && make build)
    
    echo "Running test..."
    (cd "${SDK_GO_DIR}" && make test)
    
    echo "Running coverage..."
    (cd "${SDK_GO_DIR}" && make coverage)
    
    echo "Running examples..."
    (cd "${SDK_GO_DIR}" && make examples)
else
    # Run the specified command
    echo "Running ..."
    (cd "${SDK_GO_DIR}" && make "")
fi

# Print success message
echo "[ok]  completed successfully ✔️"
