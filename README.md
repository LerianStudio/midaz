![banner](image/README/midaz-banner.png)

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/LerianStudio/midaz?include_prereleases)](https://github.com/LerianStudio/midaz/releases)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://github.com/LerianStudio/midaz/blob/main/LICENSE)
[![Go Report](https://goreportcard.com/badge/github.com/lerianstudio/midaz)](https://goreportcard.com/report/github.com/lerianstudio/midaz)
[![Discord](https://img.shields.io/badge/Discord-Lerian%20Studio-%237289da.svg?logo=discord)](https://discord.gg/DnhqKwkGv3)

</div>

# Midaz: Next-Gen Open-Source Ledger

Midaz is an open-source ledger system that is part of a Core Banking Platform being developed by Lerian. The ledger and the other components of the platform are currently UNDER CONSTRUCTION and should not be used in production.

Midaz is designed to address the limitations of traditional ledger systems and provide a comprehensive, multi-asset, multi-currency, and immutable ledger solution for the modern financial landscape.

## Understanding Core Banking

At Lerian, we view a core banking system as a comprehensive platform consisting of four main components:

1. **Ledger (Core)**: The central database that manages all transactions and accounts. This is where Midaz plays a crucial role, serving as the foundation of the core banking system.

2. **Transactional Services**: These generate debits and credits in the ledger. Examples include instant payments (like PIX in Brazil), card transactions, and wire transfers.

3. **Governance**: This component includes integrations for onboarding, anti-fraud/AML measures, management reporting, regulatory compliance, and accounting.

4. **Connectivity Infrastructure**: This provides the necessary (if any, since most advanced regional financial systems are moving to the cloud) physical connections to external systems and networks.

Our open-source approach allows developers to integrate Midaz seamlessly with other components, creating a complete core banking solution tailored to their specific needs. Whether you're building a new financial product, modernizing legacy systems, or creating innovative fintech solutions, Midaz provides the solid foundation you need for your core banking infrastructure.

Lerian is committed to also providing a robust set of Transactional Services and Governance integrations to complement Midaz. More to come soon.

## Key Features

- **Open-source under Apache 2.0 License**: Freely available for developers to access, modify, and contribute.
- **Double-entry chart-of-accounts engine**: Ensures financial integrity and accuracy.
- **Multi-asset and multi-currency support**: Seamlessly handle transactions across various currencies and asset types.
- **Complex "n:n" transactions**: Efficiently process transactions involving multiple senders and receivers.
- **Native immutability and auditability**: Tamper-proof transaction records for enhanced security and compliance.
- **Cloud-native and cloud-agnostic design**: Flexible deployment across various cloud environments.
- **API-based + CLI and console**: Easy integration and interaction with existing systems.
- **Advanced security measures**: Including encryption and double-token authentication.
- **SOC-2, GDPR and PCI-DSS ready**: Adherence to strict security and compliance standards.
- **Customizable governance flows**: Define and implement custom transaction approval workflows.
- **Proprietary DSL**: For efficient transaction modeling and customization.
- **Smart contract integration**: Enabling the programming of complex types of transactions, as well as automated compliance.

## Getting Started

To begin using Midaz, please follow our [Getting Started Guide](https://docs.midaz.io/getting-started).

## Documentation

For comprehensive documentation on Midaz features, API references, and best practices, visit our [Official Documentation](https://docs.midaz.io).

## Community and Support

- Join our [Discord community](https://discord.gg/DnhqKwkGv3) for discussions, support, and updates.
- For bug reports and feature requests, please use our [GitHub Issues](https://github.com/LerianStudio/midaz/issues).
- Follow us on [Twitter](https://twitter.com/LerianStudio) for the latest news and announcements.

## Contributing

We welcome contributions from the community! Please read our [Contributing Guidelines](CONTRIBUTING.md) to get started.

## License

Midaz is released under the Apache License 2.0. See [LICENSE](LICENSE) for more information. In a nutshell, this means you can use, modify, and distribute Midaz as you see fit, as long as you include the original copyright and license notice.

## About Lerian

Midaz is developed by Lerian, a tech company founded in 2023, led by a team with a track record in developing ledger and core banking solutions.

## Contact

For any inquiries or support, please reach out to us at [contact@lerian.io](mailto:contact@lerian.io) or simply open a Discussion in our [GitHub repository](https://github.com/LerianStudio/midaz/discussions).
