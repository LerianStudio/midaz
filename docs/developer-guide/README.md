# Developer Guide

**Navigation:** [Home](../) > Developer Guide

This section provides comprehensive guidance for developers working with the Midaz codebase. Whether you're a new contributor or an experienced developer integrating with Midaz, this guide contains essential information about the codebase structure, development practices, and technical standards.

## Core Development Guides

- [Code Organization](./code-organization.md) - Overview of the project structure and organization
- [Contributing](./contributing.md) - Guidelines for contributing to the Midaz project
- [Error Handling](./error-handling.md) - Approach to error handling across the codebase
- [Shared Packages](./shared-packages.md) - Overview of shared packages and utilities
- [Testing Strategy](./testing-strategy.md) - Approach to testing, including unit, integration, and end-to-end tests

## Authentication

Authentication in Midaz is handled by a separate plugin that can be enabled or disabled through configuration. The Auth Plugin documentation is maintained in a separate repository.

> **Note:** When enabled (`PLUGIN_AUTH_ENABLED=true`), all API requests require authentication using OAuth 2.0 Bearer tokens. If authentication is disabled (`PLUGIN_AUTH_ENABLED=false`), API endpoints can be accessed without authentication, which is typically only used for development and testing environments.

## Development Workflows

### Setting Up Development Environment

1. Clone the repository
2. Set up the required dependencies (Go 1.19+, Docker, Docker Compose)
3. Configure environment variables using the provided `.env.example` files
4. Run the infrastructure services using Docker Compose
5. Run the application services

Detailed setup instructions are available in the [Installation Guide](../getting-started/installation.md).

### Development Best Practices

- Follow the code style and organization patterns in existing code
- Write comprehensive tests for all new features
- Document your code and APIs
- Use the error handling patterns described in [Error Handling](./error-handling.md)
- Create clear and descriptive commit messages

### Pull Request Process

1. Create a feature branch from the main branch
2. Implement your changes following the coding standards
3. Write or update tests to cover your changes
4. Update documentation as necessary
5. Submit a pull request with a clear description of the changes
6. Address any feedback from code reviewers

## Additional Resources

- [API Reference](../api-reference/README.md) - Comprehensive API documentation
- [Architecture Overview](../architecture/system-overview.md) - High-level system architecture
- [Tutorials](../tutorials/README.md) - Step-by-step guides for common development tasks
