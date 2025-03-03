#!/bin/bash

# Path to the repository interfaces
REPO_DIR="./internal/domain/repository"

# Generate mocks for assetrate repository
mockgen -source=${REPO_DIR}/assetrate.go -destination=${REPO_DIR}/assetrate_mock.go -package=repository

# Generate mocks for balance repository
mockgen -source=${REPO_DIR}/balance.go -destination=${REPO_DIR}/balance_mock.go -package=repository

# Generate mocks for operation repository
mockgen -source=${REPO_DIR}/operation.go -destination=${REPO_DIR}/operation_mock.go -package=repository

# Generate mocks for transaction repository
mockgen -source=${REPO_DIR}/transaction.go -destination=${REPO_DIR}/transaction_mock.go -package=repository

echo "Mocks generated successfully!"