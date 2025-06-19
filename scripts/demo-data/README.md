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

The application supports multiple configuration sources:

1. Environment variables (prefix: `DEMO_DATA_`)
2. Configuration files (YAML/JSON)
3. Command-line flags
4. Default values

### Environment Variables

- `DEMO_DATA_API_BASE_URL` - Midaz API base URL
- `DEMO_DATA_AUTH_TOKEN` - Authentication token
- `DEMO_DATA_DEBUG` - Enable debug mode
- `DEMO_DATA_LOG_LEVEL` - Log level (debug, info, warn, error)

## Project Status

This is the foundation setup (MT-001) which includes:

- ✅ Hexagonal Architecture structure
- ✅ Build system with Makefile
- ✅ Basic domain interfaces
- ✅ Dependency injection container
- 🚧 Configuration system (next phase)
- 🚧 SDK integration (next phase)
- 🚧 CLI framework (next phase)
- 🚧 Data generation (future phases)

## Contributing

1. Follow Go conventions and project style
2. Run quality checks before committing
3. Ensure tests pass
4. Update documentation as needed

## License

This project is part of the Midaz ecosystem.