# MDZ - Midaz CLI

## Overview

**MDZ** is the official command-line interface for the Midaz ledger platform. It provides a user-friendly way to interact with Midaz APIs, manage entities, and perform administrative tasks without writing code or using HTTP clients.

## Purpose

MDZ enables users to:

- **Manage organizations**: Create, list, update, delete organizations
- **Manage ledgers**: Create and configure ledgers
- **Manage assets**: Define currencies, cryptocurrencies, and commodities
- **Manage accounts**: Create and organize financial accounts
- **Manage portfolios**: Group accounts for reporting
- **Manage segments**: Create logical divisions
- **Authenticate**: Login with OAuth/JWT
- **Configure**: Set API endpoints and credentials

## Architecture

```
mdz/
├── main.go                 # Application entry point
├── internal/               # Internal implementation
│   ├── domain/            # Domain layer
│   │   └── repository/    # Repository interfaces
│   ├── model/             # Data models
│   └── rest/              # REST API clients
├── pkg/                    # Public packages
│   ├── cmd/               # Command implementations
│   ├── environment/       # Environment configuration
│   ├── factory/           # Dependency injection
│   ├── iostreams/         # I/O abstractions
│   ├── output/            # Output formatting
│   ├── setting/           # Settings persistence
│   └── tui/               # Terminal UI components
└── test/                   # Integration tests
```

## Commands

### Authentication

```bash
# Login to Midaz
mdz login

# Login with specific auth server
mdz login --auth-url https://auth.midaz.io

# Configure API endpoints
mdz configure
```

### Organizations

```bash
# Create organization
mdz organization create --legal-name "Acme Corp" --legal-document "12345678"

# List organizations
mdz organization list

# Describe organization
mdz organization describe --organization-id <uuid>

# Update organization
mdz organization update --organization-id <uuid> --legal-name "New Name"

# Delete organization
mdz organization delete --organization-id <uuid>
```

### Ledgers

```bash
# Create ledger
mdz ledger create --organization-id <uuid> --name "Main Ledger"

# List ledgers
mdz ledger list --organization-id <uuid>

# Describe ledger
mdz ledger describe --organization-id <uuid> --ledger-id <uuid>

# Update ledger
mdz ledger update --organization-id <uuid> --ledger-id <uuid> --name "Updated"

# Delete ledger
mdz ledger delete --organization-id <uuid> --ledger-id <uuid>
```

### Assets

```bash
# Create asset
mdz asset create \
  --organization-id <uuid> \
  --ledger-id <uuid> \
  --name "US Dollar" \
  --type currency \
  --code USD

# List assets
mdz asset list --organization-id <uuid> --ledger-id <uuid>

# Describe asset
mdz asset describe \
  --organization-id <uuid> \
  --ledger-id <uuid> \
  --asset-id <uuid>
```

### Accounts

```bash
# Create account
mdz account create \
  --organization-id <uuid> \
  --ledger-id <uuid> \
  --name "Customer Account" \
  --asset-code USD \
  --alias @customer1

# List accounts
mdz account list --organization-id <uuid> --ledger-id <uuid>

# Describe account
mdz account describe \
  --organization-id <uuid> \
  --ledger-id <uuid> \
  --account-id <uuid>
```

### Portfolios

```bash
# Create portfolio
mdz portfolio create \
  --organization-id <uuid> \
  --ledger-id <uuid> \
  --name "Corporate Portfolio"

# List portfolios
mdz portfolio list --organization-id <uuid> --ledger-id <uuid>
```

### Segments

```bash
# Create segment
mdz segment create \
  --organization-id <uuid> \
  --ledger-id <uuid> \
  --name "North America"

# List segments
mdz segment list --organization-id <uuid> --ledger-id <uuid>
```

### Utility Commands

```bash
# Show version
mdz version

# Show help
mdz --help
mdz <command> --help

# Disable colored output
mdz --no-color <command>

# Verbose output
mdz --verbose <command>
```

## Installation

### From Source

```bash
cd components/mdz
make build
make install-local
```

### From Release

```bash
# Download binary from GitHub releases (latest version)
curl -L https://github.com/LerianStudio/midaz/releases/latest/download/mdz-<os>-<arch> -o mdz
chmod +x mdz
sudo mv mdz /usr/local/bin/
```

### Via Package Manager

```bash
# Homebrew (macOS)
brew install lerianstudio/tap/mdz

# Chocolatey (Windows)
choco install mdz
```

## Configuration

### Settings File

MDZ stores configuration in `~/.midaz/settings.json`:

```json
{
  "client_id": "your-client-id",
  "client_secret": "your-client-secret",
  "url_api_auth": "https://auth.midaz.io",
  "url_api_ledger": "https://api.midaz.io",
  "token": "jwt-token"
}
```

### Environment Variables

- `MIDAZ_API_URL`: Override API endpoint
- `MIDAZ_AUTH_URL`: Override auth endpoint
- `MIDAZ_CLIENT_ID`: OAuth client ID
- `MIDAZ_CLIENT_SECRET`: OAuth client secret

## Features

### 1. Interactive Mode

MDZ provides interactive prompts for missing required fields:

- Uses terminal UI (TUI) components
- Password masking
- Selection menus
- Input validation

### 2. Output Formats

**Human-Readable (Default):**

- Colored output
- Formatted tables
- Progress indicators

**JSON:**

- Machine-readable
- Scriptable
- Use `--output json` flag

### 3. Authentication

**OAuth Flow:**

1. Run `mdz login`
2. Opens browser for authentication
3. Receives OAuth token
4. Stores token in settings file
5. Token used for subsequent requests

**Token Management:**

- Automatic token refresh
- Secure storage
- Expiration handling

### 4. Error Handling

- User-friendly error messages
- HTTP status code translation
- Validation error details
- Retry suggestions

### 5. Testing

**Unit Tests:**

- Command logic tests
- Mock repositories
- Golden file tests

**Integration Tests:**

- End-to-end CLI tests
- Real API calls
- Shell script tests

## Architecture Layers

### Internal Layer

**domain/repository:**

- Repository interfaces for each entity
- Abstracts REST API calls
- Defines data access contract

**model:**

- Data models for CLI
- Authentication models
- Error models

**rest:**

- REST API client implementations
- HTTP request/response handling
- Error mapping

### Package Layer

**cmd:**

- Cobra command definitions
- Flag parsing
- Command execution logic
- Organized by entity (organization, ledger, account, etc.)

**environment:**

- Environment configuration
- Version information
- API URLs

**factory:**

- Dependency injection
- Repository instantiation
- IOStreams configuration

**iostreams:**

- I/O abstractions
- Testable input/output
- Stream redirection

**output:**

- Output formatting
- JSON marshaling
- Colored output
- Error formatting

**setting:**

- Settings file management
- Credential storage
- Configuration persistence

**tui:**

- Terminal UI components
- Interactive prompts
- Input validation
- Selection menus

## Command Structure

Each entity command follows the same pattern:

```
mdz <entity> <action> [flags]

Actions:
- create    Create new entity
- list      List entities
- describe  Show entity details
- update    Update entity
- delete    Delete entity
```

### Common Flags

```
--organization-id    Organization UUID (required for most commands)
--ledger-id          Ledger UUID (required for ledger-scoped entities)
--output             Output format (table, json)
--no-color           Disable colored output
--verbose            Enable verbose logging
--help, -h           Show help
```

## Development

### Building

```bash
make build
```

### Testing

```bash
# Unit tests
make test

# Integration tests
make test_integration

# Coverage
make cover
```

### Linting

```bash
make lint
make format
```

## Dependencies

### Core Libraries

- **cobra**: CLI framework
- **viper**: Configuration management
- **color**: Terminal colors
- **tablewriter**: Table formatting

### Midaz Libraries

- **pkg/mmodel**: Domain models
- **pkg/net/http**: HTTP utilities

## Usage Examples

### Complete Workflow

```bash
# 1. Login
mdz login

# 2. Create organization
ORG_ID=$(mdz organization create \
  --legal-name "Acme Corp" \
  --legal-document "12345678" \
  --output json | jq -r '.id')

# 3. Create ledger
LEDGER_ID=$(mdz ledger create \
  --organization-id $ORG_ID \
  --name "Main Ledger" \
  --output json | jq -r '.id')

# 4. Create asset
mdz asset create \
  --organization-id $ORG_ID \
  --ledger-id $LEDGER_ID \
  --name "US Dollar" \
  --type currency \
  --code USD

# 5. Create account
mdz account create \
  --organization-id $ORG_ID \
  --ledger-id $LEDGER_ID \
  --name "Customer Account" \
  --asset-code USD \
  --alias @customer1

# 6. List accounts
mdz account list \
  --organization-id $ORG_ID \
  --ledger-id $LEDGER_ID
```

### Scripting

```bash
#!/bin/bash
# Bulk account creation

ORG_ID="..."
LEDGER_ID="..."

for i in {1..100}; do
  mdz account create \
    --organization-id $ORG_ID \
    --ledger-id $LEDGER_ID \
    --name "Account $i" \
    --asset-code USD \
    --alias @account$i \
    --output json
done
```

## Testing

### Golden File Tests

MDZ uses golden file tests for output validation:

- Expected output stored in `testdata/*.golden`
- Actual output compared against golden files
- Ensures consistent CLI output

### Integration Tests

```bash
# Run integration tests
cd test/integration
./mdz.sh
```

## Troubleshooting

### Authentication Issues

```bash
# Check settings
cat ~/.midaz/settings.json

# Re-login
mdz login

# Verify token
mdz organization list
```

### API Connection Issues

```bash
# Check API endpoint
mdz configure

# Test connection
curl -I https://api.midaz.io/health
```

### Common Errors

**"Try the login command first":**

- Run `mdz login` to authenticate

**"Organization not found":**

- Verify organization ID with `mdz organization list`

**"Invalid UUID":**

- Check UUID format (must be valid UUIDv4/v7)

## Best Practices

### Security

- Don't commit settings.json
- Use environment variables in CI/CD
- Rotate credentials regularly
- Use least-privilege accounts

### Scripting

- Use `--output json` for parsing
- Check exit codes
- Handle errors gracefully
- Use jq for JSON processing

### Performance

- Use list commands with pagination
- Cache organization/ledger IDs
- Batch operations when possible

## Contributing

See CONTRIBUTING.md in the root directory.

## Support

- GitHub Issues: https://github.com/LerianStudio/midaz/issues
- Documentation: https://docs.midaz.io
- Community: https://discord.gg/midaz

## License

See LICENSE file in the root directory.
