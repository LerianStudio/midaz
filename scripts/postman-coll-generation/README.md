# Postman Collection Generation

This directory contains all the scripts and dependencies needed to generate Postman collections from OpenAPI/Swagger documentation.

## Overview

The documentation generation process has been enhanced with bulletproofing features to ensure reliable execution in CI/CD pipelines and local development environments.

## Files

- **`convert-openapi.js`** - Converts OpenAPI specs to Postman collections with enhanced examples
- **`create-workflow.js`** - Creates workflow sequences from WORKFLOW.md files  
- **`convert-swagger.js`** - Native Node.js converter from Swagger JSON to OpenAPI YAML
- **`sync-postman.sh`** - Orchestrates the conversion and merging of Postman collections
- **`package.json`** - Node.js dependencies for all conversion scripts

## Usage

**Simple one-command usage:**
```bash
make generate-docs
```

This single command will:
1. ✅ Set up all required dependencies automatically
2. ✅ Generate Swagger JSON files for all components
3. ✅ Convert to OpenAPI YAML format
4. ✅ Create Postman collections with proper examples
5. ✅ Merge collections and add workflow sequences
6. ✅ Validate all outputs with comprehensive checks

## Features

### 🛡️ Bulletproofing Features

- **Retry Logic**: Automatic retry with exponential backoff for transient failures
- **Atomic Operations**: Race condition prevention with file locking
- **Comprehensive Validation**: JSON/YAML structure validation and content checks
- **Dependency Management**: Automatic dependency installation and verification
- **Health Monitoring**: Pre-flight checks and post-generation validation

### 🚀 Performance Optimizations

- **Parallel Processing**: Components processed simultaneously
- **Native Conversion**: Node.js conversion preferred over Docker
- **Optimized Dependencies**: Fast npm ci installation
- **Efficient Merging**: Single-pass jq operations for collection merging

### 📋 Generated Outputs

The process generates the following files:

**Per Component:**
- `components/{component}/api/swagger.json` - Swagger API specification
- `components/{component}/api/openapi.yaml` - OpenAPI 3.0 specification

**Merged Collections:**
- `postman/MIDAZ.postman_collection.json` - Complete Postman collection
- `postman/MIDAZ.postman_environment.json` - Environment template
- `postman/backups/` - Timestamped backups of previous versions

## Architecture

```
make generate-docs
├── setup-deps.sh                 # Dependency setup
└── generate-docs.sh              # Main orchestrator
    ├── Parallel Processing
    │   ├── onboarding component
    │   └── transaction component
    ├── convert-swagger.js         # Format conversion
    └── sync-postman.sh            # Postman generation
        ├── convert-openapi.js     # OpenAPI → Postman
        └── create-workflow.js     # Workflow sequences
```

## Troubleshooting

### Common Issues

**1. "Required tool not found"**
```bash
# Install missing tools:
brew install node jq go
go install github.com/swaggo/swag/cmd/swag@latest
```

**2. "Node.js dependencies not installed"**
```bash
# Dependencies are installed automatically, but if needed:
cd scripts/postman-coll-generation && npm install
```

**3. "Lock file exists"**
```bash
# Remove stale lock files:
rm -f tmp/locks/*.lock tmp/setup-docs.lock
```

### Debug Mode

For detailed logging, check the temporary files:
```bash
# View component-specific logs
cat tmp/onboarding.out
cat tmp/onboarding.err

# View sync logs  
cat tmp/sync.out
cat tmp/sync.err
```

### Manual Steps (if needed)

If the automated process fails, you can run components manually:
```bash
# 1. Setup dependencies
scripts/setup-deps.sh

# 2. Generate documentation
scripts/generate-docs.sh
```

## CI/CD Integration

The system is designed for reliable CI/CD usage:

```yaml
# GitHub Actions example
- name: Generate API Documentation
  run: make generate-docs
```

**Features for CI:**
- ✅ Zero-configuration setup
- ✅ Comprehensive error reporting
- ✅ Atomic file operations
- ✅ Parallel processing for speed
- ✅ Detailed validation and health checks

## Development

### Adding New Components

To add a new component to the documentation generation:

1. Add component name to `COMPONENTS` array in `generate-docs.sh`
2. Ensure component has `cmd/app/main.go` with Swagger annotations
3. Test with `make generate-docs`

### Customizing Output

- **Request Examples**: Modify `convert-openapi.js` → `generateExampleFromSchema()`
- **Workflow Sequences**: Edit `postman/WORKFLOW.md`
- **Environment Variables**: Update `createEnvironmentTemplate()` in `convert-openapi.js`

## Monitoring

The system provides comprehensive monitoring:

- **Pre-flight Checks**: System dependencies, disk space, component structure
- **Real-time Status**: Atomic status updates during processing
- **Post-generation Validation**: Output file validation and structure checks
- **Performance Metrics**: Timing information for each component

All checks include detailed logging for troubleshooting and monitoring in CI/CD environments.