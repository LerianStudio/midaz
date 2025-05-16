# Midaz Demo Data Generator

A realistic data generator for the Midaz platform using Faker.js and the TypeScript SDK. This tool creates hierarchical financial data with Brazilian-specific attributes following the standard entity relationships:

`Organizations → Ledgers → Assets → Portfolios → Segments → Accounts → Transactions`

## Features

- Generates realistic Brazilian financial data (70% PF / 30% PJ with valid CPF/CNPJ)
- Supports configurable volume sizes (small, medium, large)
- Uses standard Midaz SDK methods for all API interactions
- Maintains relationship integrity between entities
- Supports idempotent execution (safe to run multiple times)
- Includes exponential backoff retry for transient errors
- Comprehensive logging and metrics
- Configurable randomness with seed support for reproducible runs

## Implementation Details

### Error Handling

The generator implements robust error handling to manage API interactions with the Midaz services:

- Exponential backoff retry for transient network errors
- Proper handling of validation errors from the Onboarding and Transaction services
- Recognition of specific error types returned by Midaz APIs (EntityNotFoundError, ValidationError, EntityConflictError, etc.)

### State Management

A singleton `StateManager` class keeps track of all generated entities and their relationships.

## Setup

1. Install dependencies:

```bash
cd scripts/demo-data
npm install
```

2. Build the project:

```bash
npm run build
```

## Usage

### Using Makefile Targets

The simplest way to run the data generator is using the provided Makefile targets:

```bash
# Generate a small volume of data (default)
make generate

# Generate a specific volume size
make generate-small
make generate-medium
make generate-large

# Build the project first then generate data
make build generate
```

### Using the Shell Script

You can also use the provided shell script directly:

```bash
# Make the script executable if needed
chmod +x run-generator.sh

# Run with a specific volume size
./run-generator.sh small
./run-generator.sh medium
./run-generator.sh large
```

### CLI Options

```
Usage: npm run generate -- [options]

Options:
  --volume, -v <size>             Data volume size (small, medium, large) (default: "small")
  --base-url, -u <url>            API base URL (default: "http://localhost")
  --onboarding-port, -o <port>    Onboarding service port (default: 3000)
  --transaction-port, -t <port>   Transaction service port (default: 3001)
  --concurrency, -c <level>       Concurrency level (default: 1)
  --debug, -d                     Enable debug mode (default: false)
  --auth-token, -a <token>        Authentication token
  --seed, -s <value>              Random seed for reproducible runs
  --help, -h                      Display help
```

### Examples

Generate a small dataset (default):

```bash
npm run generate
```

Generate a medium dataset:

```bash
npm run generate -- --volume medium
```

Generate a large dataset with debug information:

```bash
npm run generate -- --volume large --debug
```

Use specific API endpoints:

```bash
npm run generate -- --base-url http://api.example.com --onboarding-port 8080 --transaction-port 8081
```

Use with authentication token:

```bash
npm run generate -- --auth-token "your-auth-token"
```

Generate reproducible data using a seed:

```bash
npm run generate -- --seed 12345
```

### Makefile Integration

The generator is integrated into the main Midaz Makefile and can be run using:

```bash
# From the project root
make generate-demo-data VOLUME=medium
```

Available options:

- `VOLUME`: Data volume size (small, medium, large)
- `BASE_URL`: API base URL
- `ONBOARDING_PORT`: Onboarding service port
- `TRANSACTION_PORT`: Transaction service port
- `CONCURRENCY`: Concurrency level
- `DEBUG`: Enable debug mode (true/false)
- `AUTH_TOKEN`: Authentication token

## Data Schema

### Data Volume Metrics

| Entity Type   | Small | Medium | Large |
|---------------|-------|--------|-------|
| Organizations | 2     | 5      | 10    |
| Ledgers/Org   | 2     | 3      | 5     |
| Assets/Ledger | 3     | 5      | 8     |
| Portfolios    | 2     | 3      | 5     |
| Segments      | 2     | 4      | 6     |
| Accounts      | 10    | 50     | 100   |
| Tx/Account    | 5     | 10     | 20    |

### Generated Data

- **Organizations**: Mix of individuals (PF, 70%) and companies (PJ, 30%)
  - Valid CPF/CNPJ numbers
  - Brazilian addresses
  - Realistic company and individual names

- **Accounts**: Various types (deposit, savings, loans, marketplace, creditCard, external)
  - Linked to portfolios and segments
  - Unique aliases for identification

- **Transactions**: Transfer operations between accounts
  - Realistic BRL amounts
  - Metadata with transaction types and descriptions
  - Automatic deposit handling for accounts with insufficient balance

## Development

### Project Structure

```
/scripts/demo-data/
├── src/
│   ├── generators/       # Entity generators
│   ├── services/         # SDK client and utilities
│   ├── utils/            # Helper functions
│   ├── generator.ts      # Main orchestration
│   ├── config.ts         # Configuration settings
│   ├── types.ts          # Type definitions
│   └── index.ts          # CLI entry point
├── dist/                 # Compiled JavaScript
└── README.md             # Documentation
```

### Adding New Features

To add new entity types or customize data generation:

1. Add new generator implementations in `src/generators/`
2. Update volume metrics in `src/config.ts`
3. Modify the main orchestration in `src/generator.ts`
