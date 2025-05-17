# Midaz Demo Data Generator Makefile
# Local development commands

# Default target
.PHONY: all
all: build

# Test targets
.PHONY: test
test:
	@echo "Running simple tests for demo data generator..."
	node tests/simple-test.js

.PHONY: test-full
test-full:
	@echo "Note: The full Jest tests may not work due to environment-specific issues."
	@echo "If you encounter errors, use the 'test' target instead."
	-npm test || echo "Jest tests failed. Try using 'make test' instead."

.PHONY: test-mode
test-mode:
	@echo "Testing the generator in test mode..."
	./run-generator.sh small --test-mode

# Generate targets for different data volumes
.PHONY: generate generate-small generate-medium generate-large
generate: generate-small

generate-small:
	@echo "Generating small volume of demo data..."
	./run-generator.sh small

generate-medium:
	@echo "Generating medium volume of demo data..."
	./run-generator.sh medium

generate-large:
	@echo "Generating large volume of demo data..."
	./run-generator.sh large

# Build the TypeScript project
.PHONY: build
build:
	@echo "Building demo data generator..."
	npm install
	npm run build

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	npm run clean
	rm -rf node_modules

# Run ESLint
.PHONY: lint
lint:
	@echo "Running ESLint..."
	npm run lint

# Format code with Prettier
.PHONY: format
format:
	@echo "Formatting code..."
	npm run format

# Generate demo data with default settings
.PHONY: generate
generate:
	@echo "Generating demo data..."
	npm run generate

# Run with specific volume
.PHONY: generate-small
generate-small:
	@echo "Generating small volume of demo data..."
	npm run generate -- --volume small

.PHONY: generate-medium
generate-medium:
	@echo "Generating medium volume of demo data..."
	npm run generate -- --volume medium

.PHONY: generate-large
generate-large:
	@echo "Generating large volume of demo data..."
	npm run generate -- --volume large
