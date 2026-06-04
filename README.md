![banner](image/README/midaz-banner.png) 

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/LerianStudio/midaz?include_prereleases)](https://github.com/LerianStudio/midaz/releases)
[![License: Elastic-2.0](https://img.shields.io/badge/License-Elastic_2.0-blue.svg)](https://github.com/LerianStudio/midaz/blob/main/LICENSE)
[![Go Report](https://goreportcard.com/badge/github.com/lerianstudio/midaz)](https://goreportcard.com/report/github.com/lerianstudio/midaz)
[![Discord](https://img.shields.io/badge/Discord-Lerian%20Studio-%237289da.svg?logo=discord)](https://discord.gg/DnhqKwkGv3)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/LerianStudio/midaz)

</div>

# Lerian Midaz: Enterprise-Grade Source-Available Ledger System

Lerian Midaz is a modern, source-available ledger system for building financial infrastructure. It scales from fintech startups to enterprise banking solutions. The robust architecture empowers developers to create sophisticated financial applications that handle complex transactional requirements.

## Why Midaz?

- **Enterprise-Ready**: Built with the reliability, scalability, and security needed for mission-critical financial systems
- **Developer-Friendly**: Clean architecture and comprehensive API documentation for rapid integration and development
- **Future-Proof Design**: Multi-asset and multi-currency support to handle both traditional and digital assets in a single system
- **Community-Backed**: Growing developer community with commercial support options available from Lerian

## Core Banking

At Lerian, we view a core banking system as a comprehensive platform consisting of four main components:

1. **Ledger**: The central database that manages all transactions and accounts. This is where Midaz plays a crucial role, serving as the foundation of the core banking system. The ledger ships as a single unified binary that composes several route surfaces in-process:

   - **Onboarding**: Manages organizations, ledgers, assets, portfolios, segments, and accounts.
   - **Transaction**: Handles complex n:n transactions with double-entry accounting.
   - **CRM**: Manages Holders (natural/legal persons) and Aliases (financial identifiers like bank accounts, PIX keys).
   - **Fees**: In-process fee calculation and billing-package management embedded in the transaction flow.
2. **Transactional Messaging Integrations**: These are responsible for integrating with external systems to generate debits and credits in the ledger. Examples include instant payments (like PIX in Brazil), card transactions, and wire transfers.
3. **Governance Integrations**: These are responsible for enhancing the core banking capabilities with KYC, anti-fraud/AML measures, management reporting, regulatory compliance, and accounting reporting.

Our source-available approach allows for the integration of Midaz with other components, like transactional messaging and governance, creating a complete core banking solution tailored to your specific needs. We also provide a marketplace with different plugins that streamline the integration of these messaging systems and governance players. These plugins are built by both Lerian and the community/partners.

Interested in contributing to plugin development? Reach out in the Discussions tab or at [contact@lerian.studio](mailto:contact@lerian.studio).

## Core Architecture

Lerian Midaz is built as a modern, cloud-native platform. The core ledger is a single unified binary serving onboarding, transaction, CRM, and fees on one port (:3002), with co-located companion deploy units (tracer for real-time transaction validation, reporter-manager and reporter-worker for async report generation) and a shared infrastructure layer:

### Domains

Lerian Midaz implements a comprehensive financial hierarchy:

- **Organizations**: Top-level entities, optionally with parent-child relationships
- **Ledgers**: Financial record-keeping systems belonging to organizations
- **Assets**: Different types of value (currencies, securities, etc.) with specific codes
- **Portfolios**: Collections of accounts for organizational purposes
- **Segments**: Categories for grouping accounts (e.g., by department, product line)
- **Accounts**: Basic units for tracking financial resources, linked to assets with specific types
- **Transactions**: Financial transactions with debits and credits
- **Balances**: Account balance tracking with available funds management and transaction capabilities

### Deploy Units

The **Ledger** is the main deploy unit: one unified binary on :3002 that composes onboarding, transaction, CRM, and fees in-process. It implements hexagonal architecture with the CQRS pattern, exposes RESTful APIs with OpenAPI documentation, and uses PostgreSQL for primary data and MongoDB for flexible metadata. Its route surfaces:

- **Onboarding**: Manages the full financial hierarchy from organizations to accounts.
- **Transaction**: Handles complex n:n transactions with double-entry accounting, multiple creation methods (JSON, DSL, inflow, outflow, annotation), asset rate management, balance tracking with optimistic concurrency, and event-driven processing using RabbitMQ.
- **CRM**: Customer Relationship Management for financial services — Holders (natural/legal persons) and Aliases (financial identifiers like bank accounts, PIX keys). Folded into the ledger binary (registered under the `plugin-crm` authorization namespace), it provides field-level encryption (AES-GCM) for personal data and searchable hashing (HMAC-SHA256) for filtered queries without exposing plaintext.
- **Fees**: Fee calculation and billing-package management embedded in the ledger; fees run in-process via the transaction creation seam.

Co-located companion deploy units (each a separate Go service built from the single root module, but not folded into the ledger binary):

- **Tracer** (:4020): Real-time transaction validation and fraud-prevention API — CEL rule engine, multi-scope spending limits, and a hash-chained immutable audit trail for compliance.
- **Reporter Manager** (:4005): REST API for templates, reports, and deadlines; dispatches report-generation jobs onto a RabbitMQ work queue.
- **Reporter Worker** (:4006): Headless RabbitMQ consumer that renders reports (HTML/CSV/JSON), converts them to PDF, and stores artifacts in S3-compatible object storage; exposes a health endpoint only.

**Infrastructure Layer**: A single consolidated set of containerized infrastructure services.

- PostgreSQL with primary-replica setup for high availability
- MongoDB replica set for metadata storage
- RabbitMQ for message queuing with predefined exchanges
- Valkey for caching and message passing
- Grafana/OpenTelemetry for comprehensive monitoring

### Transaction Processing

Lerian Midaz implements true double-entry accounting with sophisticated transaction capabilities:

- **Double-Entry Engine**: Every credit has a corresponding debit, ensuring financial integrity
- **Multi-Asset Support**: Handle transactions across different currencies with automatic rate conversion
- **Complex Transactions**: Support for n:n operations (multiple sources to multiple destinations)
- **Domain-Specific Language**: Proprietary DSL for modeling complex transactions
- **Immutable Records**: Every transaction is permanently recorded for audit purposes
- **Async Processing**: Event-driven architecture for scalable transaction handling
- **Balance Management**: Sophisticated balance tracking with available and on-hold amounts

### Technical Highlights

- **Hexagonal Architecture**: Clear separation between domain logic and external dependencies
- **CQRS Pattern**: Separate command and query responsibilities for optimized performance
- **Event-Driven Design**: Asynchronous processing using message queues for scalability
- **Domain-Specific Language**: Specialized grammar for defining complex transactions
- **Optimistic Concurrency**: Version-based concurrency control for balance updates
- **Comprehensive APIs**: RESTful endpoints with OpenAPI documentation
- **Testing**: Extensive unit and integration tests with mocking support

## Getting Started

Follow our [Getting Started Guide](https://docs.lerian.studio/en/midaz/midaz-getting-started) to begin. For features, API references, and best practices, visit our [Official Documentation](https://docs.lerian.studio).

## Community & Support

- Join our [Discord community](https://discord.gg/DnhqKwkGv3) for discussions, support, and updates.
- For bug reports and feature requests, please use our [GitHub Issues](https://github.com/LerianStudio/midaz/issues).
- If you want to raise anything to the attention of the community, open a Discussion in our [GitHub](https://github.com/LerianStudio/midaz/discussions).
- Follow us on [Twitter](https://twitter.com/LerianStudio) for the latest news and announcements.

## Repo Activity

![Alt](https://repobeats.axiom.co/api/embed/827f95068c3eb21900ed6a7191a53639481cbc75.svg "Repobeats analytics image")

## Contributing & License

We welcome contributions from the community. Read our [Contributing Guidelines](CONTRIBUTING.md) to get started. Lerian Midaz is released under the [Elastic License 2.0 (ELv2)](LICENSE) — a source-available license that allows you to use, copy, modify, and redistribute Midaz, with three primary limitations: you may not provide it to others as a managed/hosted service, circumvent its license key functionality, or remove/obscure license notices.

## About Lerian

Midaz is developed by Lerian, a tech company founded in 2024. Our team has a track record in developing ledger and core banking solutions. For inquiries or support, reach out at [contact@lerian.studio](mailto:contact@lerian.studio) or open a Discussion in our [GitHub repository](https://github.com/LerianStudio/midaz/discussions).
