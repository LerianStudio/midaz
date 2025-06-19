# Demo Data Generator

A comprehensive demo data generator for the Midaz financial ledger system following Hexagonal Architecture principles.

## Overview

This tool generates realistic financial data hierarchies including organizations, ledgers, assets, portfolios, segments, accounts, and transactions for testing and demonstration purposes.

## Architecture

The project follows Hexagonal Architecture (Ports and Adapters) pattern:

```
demo-data/
├── cmd/demo-data/           # Application entry point
├── internal/
│   ├── domain/              # Core business logic
│   │   ├── entities/        # Domain entities
│   │   ├── services/        # Domain services
│   │   └── ports/           # Interfaces (ports)
│   ├── adapters/            # External adapters
│   │   ├── primary/         # Inbound adapters (CLI, HTTP)
│   │   └── secondary/       # Outbound adapters (SDK, Config)
│   └── infrastructure/      # Infrastructure concerns
│       ├── logging/         # Logging implementation
│       ├── metrics/         # Metrics collection
│       └── di/              # Dependency injection
├── pkg/                     # Public packages
├── tests/                   # Test files
├── configs/                 # Configuration files
└── docs/                    # Documentation
```

## Prerequisites

- Go 1.22 or later
- Make (for build automation)
- Access to Midaz API endpoints

## Development Setup

1. **Install development tools:**
   ```bash
   make install-tools
   ```

2. **Download dependencies:**
   ```bash
   make deps
   ```

3. **Build the application:**
   ```bash
   make build
   ```

4. **Run quality checks:**
   ```bash
   make quality
   ```

## Building

### Development Build
```bash
make dev
```

### Production Build
```bash
make prod
```

### Cross-Platform Build
```bash
make cross-compile
```

## Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage
```

## Quality Assurance

The project includes comprehensive quality checks:

```bash
# Format code
make fmt

# Static analysis
make vet

# Linting
make lint

# Security scan
make security

# Vulnerability check
make vuln-check

# All quality checks
make quality
```

## Configuration

The application supports multiple configuration sources with the following precedence (highest to lowest):

1. Command-line flags
2. Environment variables (prefix: `DEMO_DATA_`)
3. Configuration files (YAML/JSON)
4. Default values

### Configuration File

Copy the example configuration:
```bash
cp configs/config.example.yaml config.yaml
```

Edit `config.yaml` with your settings:
```yaml
# Midaz API Configuration
api_base_url: "https://api.midaz.io"
auth_token: "your-auth-token-here"
timeout_duration: "30s"

# Application Settings
debug: false
log_level: "info"

# Volume Configuration
volume_config:
  small:
    organizations: 2
    ledgers_per_org: 2
    # ... more settings
```

### Environment Variables

- `DEMO_DATA_API_BASE_URL` - Midaz API base URL
- `DEMO_DATA_AUTH_TOKEN` - Authentication token
- `DEMO_DATA_TIMEOUT_DURATION` - Request timeout (e.g., "30s")
- `DEMO_DATA_DEBUG` - Enable debug mode (true/false)
- `DEMO_DATA_LOG_LEVEL` - Log level (debug, info, warn, error)

### Volume Configuration

The tool supports three predefined volume levels:

- **Small**: 2 orgs, 2 ledgers each, 100 accounts per ledger
- **Medium**: 5 orgs, 3 ledgers each, 500 accounts per ledger  
- **Large**: 10 orgs, 5 ledgers each, 1000 accounts per ledger

All volume parameters can be customized via configuration.

## Project Status

Current implementation status:

- ✅ **ST-MT-001-001**: Hexagonal Architecture foundation
- ✅ **ST-MT-001-002**: Configuration system with Viper
- ✅ **ST-MT-001-003**: SDK integration with mock client
- 🚧 **ST-MT-001-004**: CLI framework (next phase)
- 🚧 **ST-MT-001-005**: Logging infrastructure (next phase)
- 🚧 **ST-MT-001-006**: Integration testing (next phase)

## Contributing

1. Follow Go conventions and project style
2. Run quality checks before committing: `make quality`
3. Ensure tests pass: `make test`
4. Update documentation as needed

## License

This project is part of the Midaz ecosystem.