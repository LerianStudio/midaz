# Version History

**Navigation:** [Home](./) > Version History

This document tracks the major versions and significant changes to the Midaz platform, providing a comprehensive change log that highlights important features, breaking changes, and improvements across versions.

## Overview

Midaz follows [Semantic Versioning (SemVer)](https://semver.org/) for its release versioning. The version format is MAJOR.MINOR.PATCH:

- **MAJOR** version increments indicate incompatible API changes or significant architectural changes
- **MINOR** version increments indicate backward-compatible functionality additions
- **PATCH** version increments indicate backward-compatible bug fixes

## Version History

### v2.0.0 (2025-04-05)

A major release with significant architectural changes and breaking changes to the platform.

#### Breaking Changes

- Complete redesign of the Makefile system for improved build and deployment workflow
- Changed dependency management approach for sync postman script
- Infrastructure changes requiring migration steps from v1.x

#### Key Features

- Enhanced authentication framework with plugin auth network integration
- Improved error handling with more descriptive field validations
- Enhanced MongoDB and PostgreSQL configuration:
  - Logical replication support in PostgreSQL
  - MongoDB replica set configuration
- CI/CD improvements:
  - Migrated to golangci-lint v2
  - Organized linting by module
  - Improved GitHub Action workflows

#### Bug Fixes

- Fixed JSON metadata handling with improved support for JSON Merge Patch
- Improved error code and status standardization
- Fixed API documentation generation process
- Corrected numerous internal API responses to follow error code standards
- Enhanced RabbitMQ channel configuration

### v1.51.0 (2025-04-04)

Minor update focusing on documentation and README improvements.

#### Bug Fixes

- Updated README with accurate Midaz platform description

### v1.50.0 (2025-03-21)

Performance-focused release with improvements to transaction processing and infrastructure.

#### Key Features

- Performance optimization for balance updates
- Configurable number of workers via environment variables

#### Bug Fixes

- Added database indexes to improve query performance:
  - Added balance table indexes with reindex commands
  - Added account table indexes for performance optimization
- Improved error handling:
  - Changed pagination limit error type to validation error
  - Fixed business error validation
- Fixed metadata handling for MongoDB
- Database migration improvements:
  - Added graceful error handling for migrations
  - Fixed handling when no migration files exist
- Infrastructure changes:
  - Removed pgbouncer
  - Updated network configurations
- CI/CD improvements:
  - Enhanced Discord notifications for releases
  - Fixed workflow triggers

### v1.40.0 (2024-12-27)

CI/CD focused release with improvements to the deployment and release process.

#### Bug Fixes

- Multiple CI/CD workflow and GitHub Actions improvements
- Enhanced branch detection and PR management
- Improved automated release process

### v1.30.0 (2024-10-15)

First public release with core financial transaction functionality.

#### Key Features

- Complete double-entry bookkeeping system with transaction validation
- Basic entity management (Organization, Ledger, Asset, Segment, Portfolio, Account)
- Command Line Interface (CLI) for entity and transaction management
- Event-driven architecture with RabbitMQ messaging
- Polyglot persistence with PostgreSQL and MongoDB support
- RESTful APIs for all core services

## Migration Guides

### Migrating from v1.x to v2.0

When migrating from v1.x to v2.0, follow these steps:

1. Update your Makefile references to match the new structure
2. Migrate database schemas using the provided migration scripts
3. Update client applications to handle enhanced error responses
4. Ensure MongoDB is configured for replica set if using metadata functionality

For detailed migration assistance, contact the Midaz support team.

## Documentation Versions

Midaz documentation is versioned alongside the software releases. The current documentation reflects the latest release (v2.0.0).

For historical documentation, please refer to the GitHub repository tags or contact the development team for access to archived documentation.

---

This document is maintained by the Midaz development team. Last updated: 2025-04-07.