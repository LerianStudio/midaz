# New Developer Onboarding

**Navigation:** [Home](../) > [Tutorials](../) > New Developer Onboarding

This guide provides a comprehensive walkthrough for new developers joining the Midaz project. It covers the development environment setup, codebase structure, development workflow, and best practices.

## Table of Contents

- [Development Environment Setup](#development-environment-setup)
- [Codebase Structure Overview](#codebase-structure-overview)
- [Development Workflow](#development-workflow)
- [Common Development Tasks](#common-development-tasks)
- [Testing Strategy](#testing-strategy)
- [Debugging and Troubleshooting](#debugging-and-troubleshooting)
- [Contribution Guidelines](#contribution-guidelines)
- [Resources and References](#resources-and-references)

## Development Environment Setup

### Prerequisites

Before you begin, ensure you have the following prerequisites installed:

- **Go 1.24.1 or higher** - [Download and install from golang.org](https://golang.org/doc/install)
- **Docker and Docker Compose** - [Download and install from docker.com](https://docs.docker.com/get-docker/)
- **Git** - [Download and install from git-scm.com](https://git-scm.com/downloads)

### Initial Setup

1. **Clone the Repository**

   ```bash
   git clone https://github.com/LerianStudio/midaz
   cd midaz
   ```

2. **Set Up Development Environment**

   ```bash
   make dev-setup
   ```

   This command will:
   - Install necessary Git hooks
   - Install development tools (golangci-lint, mockgen, swag, gosec, etc.)
   - Configure your development environment

3. **Set Up Environment Variables**

   ```bash
   make set-env
   ```

   This command copies all `.env.example` files to `.env` in each component directory. Review and modify these files as needed for your environment.

4. **Start All Services**

   ```bash
   make up
   ```

   This command starts all required services using Docker Compose.

5. **Verify Installation**

   ```bash
   make logs
   ```

   Ensure all services are running without errors.

## Codebase Structure Overview

Midaz follows a modular microservices architecture with the following main components:

### Top-Level Structure

```
midaz/
├── components/           # Main microservices components
│   ├── infra/            # Infrastructure setup (databases, message queues)
│   ├── mdz/              # CLI component
│   ├── onboarding/       # Onboarding API component
│   └── transaction/      # Transaction processing component
├── pkg/                  # Shared packages used across components
│   ├── constant/         # Shared constants
│   ├── gold/             # Transaction DSL parser
│   ├── mmodel/           # Shared model definitions
│   ├── net/              # Network utilities
│   └── shell/            # Shell utilities
├── docs/                 # Documentation
├── scripts/              # Build and utility scripts
└── ...
```

### Key Components

1. **Infrastructure (`components/infra/`)**
   - Contains Docker configurations for PostgreSQL, MongoDB, RabbitMQ, and other supporting services
   - Manages infrastructure dependencies for development and testing

2. **MDZ CLI (`components/mdz/`)**
   - Command-line interface for interacting with the Midaz system
   - Built using Cobra for a modern CLI experience
   - Organized using domain-driven design patterns

3. **Onboarding Service (`components/onboarding/`)**
   - Manages financial entities (organizations, ledgers, accounts, etc.)
   - Implements hexagonal architecture with CQRS pattern
   - Uses PostgreSQL for primary data, MongoDB for flexible metadata

4. **Transaction Service (`components/transaction/`)**
   - Processes financial transactions using double-entry accounting principles
   - Follows the same architectural pattern as the onboarding service
   - Implements the Balance-Transaction-Operation (BTO) pattern

5. **Shared Packages (`pkg/`)**
   - Common code used across multiple components
   - Domain models, constants, error handling, and utilities
   - Contains the transaction Domain-Specific Language (DSL) parser

### Architectural Patterns

Each service component follows these key architectural patterns:

1. **Hexagonal Architecture**
   - Core domain logic isolated from external concerns
   - Port interfaces define interactions with external systems
   - Adapter implementations connect to specific technologies

2. **Command Query Responsibility Segregation (CQRS)**
   - Separate command (write) and query (read) operations
   - Commands in `internal/services/command/`
   - Queries in `internal/services/query/`

3. **Repository Pattern**
   - Domain entities accessed through repository interfaces
   - Technology-specific implementations (PostgreSQL, MongoDB)
   - Clean separation between business logic and persistence

For a deeper understanding of these patterns, refer to the [Hexagonal Architecture](../architecture/hexagonal-architecture.md) and [CQRS Pattern](../developer-guide/code-organization.md#cqrs-pattern) documentation.

## Development Workflow

### Git Workflow

1. **Branch Management**
   - Create a feature branch from `main`
   - Use descriptive branch names (e.g., `feature/add-new-entity`, `fix/transaction-validation`)
   - Follow the [Conventional Commits](https://www.conventionalcommits.org/) format for commit messages

2. **Pull Request Process**
   - Create a PR against the `main` branch
   - Ensure all tests pass and code quality checks are satisfied
   - Address code review feedback
   - Sign your commits with the `-s` flag

### Making Changes

1. **Locate the relevant component**
   - Determine which service (onboarding, transaction, CLI) your change affects
   - Find the appropriate domain within that service

2. **Understand the existing code**
   - Review the component's `internal/domain` directory to understand domain entities
   - Check existing implementations in the `internal/adapters` directory
   - Look at handler implementations in `internal/services/command` or `internal/services/query`

3. **Follow the established patterns**
   - Use the existing code as a reference for how to structure your changes
   - Maintain separation between commands and queries
   - Follow the repository pattern for data access
   - Keep business logic independent of infrastructure

4. **Update tests**
   - Add or update unit tests for your changes
   - Follow the established test patterns and naming conventions
   - Aim for good test coverage (checked with `make cover`)

## Common Development Tasks

### Building and Running

```bash
# Build all components
make build

# Start all services with Docker
make up

# View logs
make logs

# Rebuild and restart services
make rebuild-up
```

### Component-Specific Commands

```bash
# Execute a command for a specific component
make mdz COMMAND=build
make onboarding COMMAND=test
make transaction COMMAND=generate-docs
```

### Code Quality

```bash
# Format code
make format

# Run linter
make lint

# Security check
make sec

# Clean up dependencies
make tidy
```

### API Documentation

```bash
# Generate OpenAPI documentation
make generate-docs-all

# Validate API documentation
make validate-api-docs
```

## Testing Strategy

Midaz employs a comprehensive testing approach:

### Unit Testing

```bash
# Run all tests
make test

# Run tests with coverage report
make cover
```

### Test Patterns

1. **Mocking**
   - Each repository interface has a corresponding mock implementation (`*_mock.go`)
   - Use the mock implementations for unit testing services
   - Mock generation is handled by the `mockgen` tool

2. **Table-Driven Tests**
   - Tests use a table-driven approach with multiple test cases
   - Each test case includes input data and expected outputs
   - Descriptive test case names explain the scenario being tested

3. **Golden File Tests**
   - Used for testing CLI output and complex serialization
   - Expected outputs stored in `testdata/*.golden` files
   - Test compares actual output with the golden file

### Integration Testing

```bash
# Run integration tests for MDZ CLI
cd components/mdz
make test_integration
```

Integration tests ensure that components work correctly together, including:
- CLI commands against a running system
- API endpoints with real database interactions
- End-to-end transaction processing

## Debugging and Troubleshooting

### Viewing Logs

```bash
# View logs from all services
make logs

# View logs from a specific service
make logs SERVICE=onboarding
make logs SERVICE=transaction
```

### Database Access

```bash
# Connect to PostgreSQL
make postgres-cli

# Connect to MongoDB
make mongo-cli
```

### Common Issues

1. **Docker Issues**
   
   If you encounter issues with Docker containers:

   ```bash
   # Clean all Docker resources
   make clean-docker

   # Rebuild and restart all services
   make rebuild-up
   ```

2. **Missing Environment Files**
   
   If you see errors about missing configuration:

   ```bash
   # Re-create environment files
   make set-env
   ```

3. **Port Conflicts**
   
   If services fail to start due to port conflicts, check for other applications using the same ports. You can modify the port mappings in the `.env` files.

## Contribution Guidelines

### Code Style

- Follow Go's idiomatic style and conventions
- Use consistent naming according to the project's conventions
- Format code with `gofmt` or `make format`

### Pull Request Checklist

Before submitting a pull request, ensure:

1. Code builds successfully with `make build`
2. All tests pass with `make test`
3. Linting passes with `make lint`
4. Security checks pass with `make sec`
5. API documentation is up-to-date with `make generate-docs-all`
6. Commit messages follow the Conventional Commits format
7. You've added tests for new features or bug fixes
8. Documentation is updated if necessary

### Code Review Process

- Pull requests require at least one approval from a maintainer
- Address all code review comments and requested changes
- Automated CI checks must pass before merging

## Resources and References

### Documentation

- [Architecture Overview](../architecture/system-overview.md)
- [Hexagonal Architecture](../architecture/hexagonal-architecture.md)
- [Code Organization](../developer-guide/code-organization.md)
- [Error Handling](../developer-guide/error-handling.md)
- [Financial Model](../domain-models/financial-model.md)

### External Resources

- [Go Documentation](https://golang.org/doc/)
- [Docker Documentation](https://docs.docker.com/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [MongoDB Documentation](https://docs.mongodb.com/)
- [RabbitMQ Documentation](https://www.rabbitmq.com/documentation.html)

### Getting Help

- Create issues in the GitHub repository for bugs or feature requests
- Contact the project maintainers for questions
- Refer to the [CONTRIBUTING.md](../../CONTRIBUTING.md) file for detailed contribution guidelines