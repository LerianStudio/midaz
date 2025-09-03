#!/bin/bash

# ==================================================
# Optimized Docker Build Script for Midaz Console
# ==================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
IMAGE_NAME="midaz-console"
TAG="${1:-latest}"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"

echo -e "${BLUE}=== Midaz Console Optimized Docker Build ===${NC}"
echo -e "${YELLOW}Image: ${IMAGE_NAME}:${TAG}${NC}"
echo ""

# Check if GITHUB_TOKEN is provided
if [ -z "$GITHUB_TOKEN" ]; then
    echo -e "${YELLOW}Warning: GITHUB_TOKEN not set. Some private packages may fail to install.${NC}"
    echo "Set it with: export GITHUB_TOKEN=your_token"
    echo ""
fi

# Build the optimized image
echo -e "${BLUE}Building optimized Docker image...${NC}"
docker build \
    --build-arg GITHUB_TOKEN="$GITHUB_TOKEN" \
    --tag "${IMAGE_NAME}:${TAG}" \
    --file Dockerfile \
    ../../../

# Get image information
IMAGE_SIZE=$(docker images "${IMAGE_NAME}:${TAG}" --format "table {{.Size}}" | tail -n 1)
IMAGE_ID=$(docker images "${IMAGE_NAME}:${TAG}" --format "{{.ID}}")

echo ""
echo -e "${GREEN}=== Build Complete! ===${NC}"
echo -e "${GREEN}Image ID: ${IMAGE_ID}${NC}"
echo -e "${GREEN}Image Size: ${IMAGE_SIZE}${NC}"
echo -e "${GREEN}Image Name: ${IMAGE_NAME}:${TAG}${NC}"
echo ""

# Optional: Show build layers (uncomment to analyze)
# echo -e "${BLUE}=== Image Layers Analysis ===${NC}"
# docker history "${IMAGE_NAME}:${TAG}" --human --format "table {{.CreatedBy}}\t{{.Size}}"

echo -e "${BLUE}=== Quick Test Commands ===${NC}"
echo "Run container:"
echo "  docker run -p 8081:8081 --name midaz-console-test ${IMAGE_NAME}:${TAG}"
echo ""
echo "Test health endpoint:"
echo "  curl http://localhost:8081/api/health"
echo ""
echo "View logs:"
echo "  docker logs midaz-console-test"
echo ""
echo "Clean up test:"
echo "  docker stop midaz-console-test && docker rm midaz-console-test"
echo ""

echo -e "${GREEN}Build completed successfully!${NC}"