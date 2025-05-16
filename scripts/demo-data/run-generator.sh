#!/bin/bash
# Script to run the Midaz demo data generator

# Set up environment
echo "Setting up environment..."

# Go to script directory
cd "$(dirname "$0")"

# Check if node_modules exists and install dependencies if needed
if [ ! -d "node_modules" ]; then
  echo "Installing dependencies..."
  npm install
  
  # Create symbolic link for the SDK if it's not already installed
  if [ ! -d "node_modules/midaz-sdk-typescript" ]; then
    echo "Linking SDK..."
    mkdir -p node_modules/midaz-sdk-typescript
    ln -sf ../../midaz-sdk-typescript/src node_modules/midaz-sdk-typescript/src
  fi
fi

# Get volume size from command line or use small as default
VOLUME=${1:-small}
echo "Using volume size: $VOLUME"

# Get auth token from command line (required)
AUTH_TOKEN=${2:?"Error: Authentication token is required. Usage: ./run-generator.sh [small|medium|large] [auth-token]"}

# Run the generator directly with ts-node (skipping type checking)
echo "Running demo data generator with volume: $VOLUME..."
TS_NODE_TRANSPILE_ONLY=true TS_NODE_COMPILER_OPTIONS='{"module":"commonjs","moduleResolution":"node"}' npx ts-node src/index.ts --volume $VOLUME --auth-token "$AUTH_TOKEN"

# Inform user about customizing command
echo ""
echo "To run with different options, use:"
echo "./run-generator.sh [small|medium|large] [auth-token]"
