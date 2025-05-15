# Midaz Demo Data Generator

A tool to generate realistic demo data for the Midaz financial system.

## Overview

This tool creates a complete set of demo data for Midaz, following the natural entity hierarchy and interdependencies:

1. Organizations (both individuals and companies)
2. Ledgers
3. Assets (BRL, USD, GBP)
4. Segments
5. Portfolios
6. Accounts
7. Transactions (including initial deposits and transfers between accounts)

## Features

- Generates realistic Brazilian financial data using Faker.js
- Creates a mix of individual and company organizations
- Supports different data volumes (small, medium, large)
- Randomized but realistic metadata for financial transactions
- Concurrent API requests with rate limiting
- Visual progress indicators
- Detailed summary of created entities

## Usage

### Via Makefile (Recommended)

The simplest way to run the generator is through the Midaz Makefile:

```bash
# Generate using default settings (small volume)
make generate-demo-data

# Generate with a specific volume
make generate-demo-data VOLUME=medium

# Generate with authentication token
make generate-demo-data AUTH_TOKEN=your_token

# Combine options
make generate-demo-data VOLUME=large AUTH_TOKEN=your_token
```

### Direct Usage

You can also run the generator directly:

```bash
# Navigate to the script directory
cd scripts/demo-data

# Install dependencies (first time only)
npm install

# Run the generator with options
node src/index.js --volume medium --auth-token your_token
```

## Configuration

The generator uses the following environment variables, which can also be specified as command-line arguments:

- `AUTH_TOKEN`: Authentication token for API access
- `API_BASE_URL`: Base URL for the API (default: http://localhost)
- `ONBOARDING_PORT`: Port for the onboarding service (default: 3000)
- `TRANSACTION_PORT`: Port for the transaction service (default: 3001)

## Data Volumes

The following volumes are available:

- **small**: 2 organizations, 1 ledger per organization, 10 accounts per ledger, 5 transactions per account
- **medium**: 5 organizations, 2 ledgers per organization, 25 accounts per ledger, 15 transactions per account
- **large**: 10 organizations, 2 ledgers per organization, 50 accounts per ledger, 30 transactions per account

## Generated Data

The tool creates a complete, interconnected set of financial data:

### Organizations
- Mix of individuals (70%) and companies (30%)
- Brazilian addresses and contact information
- Valid CPF/CNPJ documents

### Accounts
- Distribution across checking, savings, investment, and loan accounts
- Realistic account naming patterns
- Segment classification

### Transactions
- Initial deposits from external sources
- PIX transactions with various types (transfer, payment, withdrawal)
- TED bank transfers
- Boleto payments with barcode and due date
- P2P platform transactions

## Development

The tool is built with Node.js and uses the following dependencies:

- Faker.js for data generation
- Axios for API communication
- Commander for CLI interface
- Ora for spinner animations
- p-limit for concurrency control

The code is organized into:

- Entity generators (organizations, ledgers, accounts, etc.)
- API client for Midaz API communication
- Orchestrator for managing the generation process
- CLI interface for user interaction

## Troubleshooting

If you encounter issues:

1. Ensure the Midaz services are running (onboarding and transaction)
2. Verify your API authentication token if required
3. Check the connection details (base URL and ports)
4. Look for error messages in the console output