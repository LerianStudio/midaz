# Frequently Asked Questions

**Navigation:** [Home](./) > FAQ

This document answers frequently asked questions about the Midaz platform. If you don't find an answer to your question here, please check the [Troubleshooting Guide](./troubleshooting.md) or [contact support](#support-and-community).

## Table of Contents

- [General Questions](#general-questions)
- [Technical Questions](#technical-questions)
- [Usage Questions](#usage-questions)
- [Deployment and Infrastructure](#deployment-and-infrastructure)
- [Integration Questions](#integration-questions)
- [Troubleshooting](#troubleshooting)
- [Development Questions](#development-questions)
- [Security Questions](#security-questions)
- [Advanced Features](#advanced-features)
- [Support and Community](#support-and-community)

## General Questions

### What is Midaz?

Midaz is an enterprise-grade open-source ledger system designed for building financial infrastructure. It provides a robust foundation for creating sophisticated financial applications ranging from fintech startups to enterprise banking solutions.

### What can I build with Midaz?

Midaz can be used to build:
- Digital banking platforms
- Payment processing systems
- Asset management solutions
- Financial inclusion applications
- Treasury management systems
- Blockchain/crypto integration platforms
- Core banking systems

### What makes Midaz different from other ledger systems?

Midaz differentiates itself through:
- **Enterprise-ready reliability**: Built for financial workloads with transactional integrity
- **Developer-friendly architecture**: Modern, modular design with clean APIs
- **Future-proof multi-asset support**: Handles traditional and digital assets in a unified system
- **Compliance-focused features**: Audit trails, governance, and regulatory reporting features
- **Comprehensive documentation**: Extensive guides and resources for all user types

### Is Midaz free to use?

Yes, Midaz is open-source software released under the Apache License 2.0, which allows you to use, modify, and distribute it freely, as long as you include the original copyright and license notice.

### Who develops Midaz?

Midaz is developed by Lerian Studio, with contributions from an active open-source community. The core team has extensive experience in developing ledger and core banking solutions.

## Technical Questions

### What is the architecture of Midaz?

Midaz uses a modular microservices architecture with:
- Hexagonal design (ports and adapters pattern)
- CQRS (Command Query Responsibility Segregation)
- Event-driven communication between services
- Domain-driven design principles
- Clear separation between domain logic and external dependencies

### What are the main components of Midaz?

The main components are:
1. **Onboarding Service** - Manages organizations, ledgers, assets, portfolios, segments, and accounts
2. **Transaction Service** - Handles transactions, operations, and balances
3. **MDZ CLI** - Command-line interface for system management
4. **Infrastructure Layer** - Containerized infrastructure services (databases, message broker, monitoring)

### What databases does Midaz use?

Midaz uses:
- **PostgreSQL** for primary data storage with a primary-replica setup
- **MongoDB** for flexible metadata storage
- **Redis/Valkey** for caching and distributed locking

### How does Midaz handle messaging?

Midaz uses RabbitMQ for message queuing with:
- Predefined exchanges for different event types
- Event-driven architecture for transaction lifecycle management
- Asynchronous processing of operations

### What programming languages are used in Midaz?

Midaz is primarily developed using Go (Golang), which provides:
- Excellent performance for financial operations
- Strong typing and compile-time checking
- Robust concurrency handling
- Cloud-native capabilities

### Does Midaz support multi-currency transactions?

Yes, Midaz has built-in multi-asset and multi-currency support through:
- Flexible asset definitions
- Asset rates for conversion
- Multi-currency transaction capabilities
- Currency-specific formatting and handling

### What is the Transaction DSL in Midaz?

Midaz includes a Domain-Specific Language (DSL) for modeling complex transactions. This lisp-like language makes it easier to define sophisticated financial operations through a specialized grammar that supports:
- Multi-source transactions
- Percentage-based distributions
- Currency conversions
- Custom metadata

## Usage Questions

### How do I set up Midaz locally?

To set up Midaz locally:
1. Ensure Docker and Docker Compose are installed
2. Clone the repository: `git clone https://github.com/LerianStudio/midaz`
3. Set up environment variables: `make set-env`
4. Start all services: `make up`

Detailed instructions are available in the [Installation Guide](./getting-started/installation.md).

### How do I interact with Midaz?

You can interact with Midaz through:
1. The MDZ CLI (command-line interface)
2. RESTful APIs exposed by the services
3. Client libraries in various programming languages

### How do I create my first financial structure?

To create a basic financial structure:
1. Create an organization
2. Create a ledger within the organization
3. Define assets (currencies)
4. Create portfolios and segments
5. Create accounts

See the [Creating Financial Structures](./tutorials/creating-financial-structures.md) tutorial for step-by-step instructions.

### How do I create a transaction in Midaz?

Transactions can be created using:
1. The Transaction Service API with JSON payload
2. The Transaction DSL for more complex transactions
3. Client libraries that abstract the API

See the [Implementing Transactions](./tutorials/implementing-transactions.md) tutorial for details.

### What are the core entity types in Midaz?

The main entity types in Midaz are:
- **Organizations** - Top-level entities representing companies or institutions
- **Ledgers** - Financial record-keeping systems within organizations
- **Assets** - Currencies or value stores (USD, EUR, BTC, etc.)
- **Portfolios** - Collections of accounts (e.g., for customers or departments)
- **Segments** - Categories for grouping accounts (e.g., business lines)
- **Accounts** - Basic units for tracking financial resources
- **Transactions** - Financial movements with debits and credits

## Deployment and Infrastructure

### What are the deployment options for Midaz?

Midaz is designed to be cloud-native and cloud-agnostic, supporting:
- Any cloud provider (AWS, GCP, Azure, etc.)
- On-premises deployment
- Hybrid cloud setups
- Kubernetes orchestration
- Docker Compose for simpler deployments

### What are the minimum system requirements?

For a basic development setup:
- 4GB RAM
- 2 CPU cores
- 20GB storage
- Docker and Docker Compose

Production requirements depend on your expected transaction volume and number of accounts.

### How does Midaz ensure high availability?

Midaz supports high availability through:
- PostgreSQL with primary-replica setup
- MongoDB replica sets
- Distributed message queuing
- Stateless service design for horizontal scaling
- Health checks and automatic recovery

### How is monitoring handled in Midaz?

Midaz includes comprehensive monitoring with:
- Grafana dashboards for visualization
- OpenTelemetry for metrics, traces, and logs collection
- Health checks for all services
- Alerting capabilities
- Performance metrics for all components

See the [Monitoring](./components/infrastructure/monitoring.md) documentation for details.

## Integration Questions

### How can I integrate Midaz with existing systems?

Midaz provides:
- Comprehensive RESTful APIs
- Webhook capabilities
- Message queue integration
- Flexible metadata for cross-system references
- Client libraries for various languages

### Does Midaz support webhooks for events?

Yes, Midaz uses an event-driven architecture that supports webhooks, allowing external systems to receive notifications about transactions and other events.

### Can Midaz integrate with blockchain or cryptocurrency systems?

Yes, Midaz is designed to bridge traditional finance and digital assets with:
- Support for crypto assets
- Flexible asset model
- Integration capabilities with blockchain systems
- Metadata for cross-chain references

### How can I extend Midaz functionality?

You can extend Midaz through:
1. Creating plugins for integrations
2. Implementing custom governance flows
3. Adding new API endpoints
4. Defining custom transaction types
5. Contributing to the core codebase

## Troubleshooting

### Why are my transactions failing?

Common reasons for transaction failures:
- Insufficient balance in source accounts
- Invalid account or entity IDs
- Asset mismatch between source and destination
- Business rule violations
- Account status issues (inactive accounts)

See the [Troubleshooting Guide](./troubleshooting.md#transaction-processing-issues) for detailed diagnostics.

### How do I debug issues in Midaz?

You can use the following approaches:
1. Check logs with `make check-logs`
2. Run tests with `make test`
3. Use the CLI with verbose flags
4. Inspect the state in databases
5. Use the monitoring dashboards

### What should I do if services won't start?

If services won't start:
1. Check Docker logs for errors
2. Verify environment variables are correctly set
3. Ensure required ports are available
4. Check disk space and system resources
5. Look for dependency issues between services

See the [Infrastructure Setup Problems](./troubleshooting.md#infrastructure-setup-problems) section in the Troubleshooting Guide.

### How do I handle transaction concurrency issues?

Midaz uses optimistic concurrency control. To handle concurrency issues:
1. Implement retry logic in applications
2. Use idempotency keys for transactions
3. Design transactions to minimize contention
4. Use the on-hold feature for funds reservation

## Development Questions

### How do I contribute to Midaz?

To contribute:
1. Fork the repository
2. Create a feature branch
3. Make your changes following the contribution guidelines
4. Sign your commits
5. Submit a pull request

See the [Contributing](./developer-guide/contributing.md) guide for detailed instructions.

### What are the coding standards for Midaz?

Midaz follows:
- Go coding conventions and best practices
- Revive for linting
- Test coverage requirements
- Conventional Commit format for commit messages
- Code review process for all changes

### How do I run tests?

Run tests with:
```bash
# Run all tests
make test

# Run specific component tests
cd components/transaction && make test
cd components/onboarding && make test
cd components/mdz && make test
```

See the [Testing Strategy](./developer-guide/testing-strategy.md) for more information.

### How is versioning handled in Midaz?

Midaz follows semantic versioning (MAJOR.MINOR.PATCH):
- Major version for incompatible API changes
- Minor version for backward-compatible features
- Patch version for backward-compatible bug fixes

Version information is accessible via the `mdz version` command or through API version endpoints.

## Security Questions

### How does Midaz handle authentication?

Midaz uses:
- Token-based authentication
- Support for OAuth 2.0
- JWT for secure token transfer
- Automatic token refresh
- Role-based access control

### What encryption does Midaz use?

Midaz ensures data security through:
- TLS for all connections
- Secure storage of sensitive information
- Password hashing with industry-standard algorithms
- Configurable encryption for data at rest

### Is Midaz compliant with financial regulations?

Midaz is designed to be SOC-2, GDPR, and PCI-DSS ready with:
- Immutable transaction records
- Comprehensive audit trails
- Access controls and permissions
- Customizable governance workflows

### How do I implement custom compliance workflows?

You can implement custom compliance workflows through:
1. Transaction approval flows
2. Metadata validation rules
3. Custom validations in transaction creation
4. Integration with external compliance systems

## Advanced Features

### How does Midaz handle double-entry accounting?

Midaz implements true double-entry accounting where:
- Every transaction ensures credits equal debits
- All operations are recorded with two sides
- Financial integrity is maintained at all times
- Audit trails preserve the full transaction history

### What is the difference between account types in Midaz?

Midaz supports various account types like:
- **Checking/Deposit** - For regular transactions
- **Savings** - For interest-bearing accounts
- **Credit Card** - For credit facilities
- **Expense** - For tracking expenses
- **Revenue** - For tracking income
- **Asset** - For tracking assets
- **Liability** - For tracking liabilities

Each type has specific behaviors and uses within the financial hierarchy.

### How do I implement multi-currency transactions?

Multi-currency transactions are implemented through:
1. Asset rates that define conversion relationships
2. Transaction DSL with conversion specifications
3. Currency-specific account configurations
4. Automatic conversion during transactions

### Can I define transaction templates?

Yes, you can create transaction templates using the Transaction DSL and reuse them with different parameters for recurring transaction patterns.

## Support and Community

### Where can I get help with Midaz?

Help resources include:
- [Documentation](https://docs.midaz.io)
- [GitHub Discussions](https://github.com/LerianStudio/midaz/discussions)
- [Discord Community](https://discord.gg/midaz)
- [Stack Overflow](https://stackoverflow.com/questions/tagged/midaz)

### How do I report a bug or security issue?

- For bugs: Create an issue on the [GitHub repository](https://github.com/LerianStudio/midaz/issues)
- For security issues: Follow the process in the [Security Policy](./SECURITY.md)

### Is commercial support available?

Yes, Lerian Studio offers commercial support options, including:
- Service level agreements (SLAs)
- Implementation assistance
- Custom development
- Training and workshops

Contact support@lerian.io for details.

### How can I stay updated on Midaz development?

Stay updated through:
- [Midaz Blog](https://midaz.io/blog)
- [Twitter/X](https://twitter.com/MidazFinance)
- GitHub repository watch/star
- Subscribing to the newsletter