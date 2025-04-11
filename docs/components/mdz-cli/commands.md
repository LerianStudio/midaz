# Midaz CLI Commands

**Navigation:** [Home](../../) > [MDZ CLI](../) > Commands

This document provides a comprehensive reference for all commands available in the Midaz CLI (`mdz`), including their syntax, options, and examples of use.

## Table of Contents

- [Global Options](#global-options)
- [Core Commands](#core-commands)
  - [login](#login)
  - [configure](#configure)
  - [version](#version)
- [Organization Management](#organization-management)
  - [organization create](#organization-create)
  - [organization list](#organization-list)
  - [organization describe](#organization-describe)
  - [organization update](#organization-update)
  - [organization delete](#organization-delete)
- [Ledger Management](#ledger-management)
  - [ledger create](#ledger-create)
  - [ledger list](#ledger-list)
  - [ledger describe](#ledger-describe)
  - [ledger update](#ledger-update)
  - [ledger delete](#ledger-delete)
- [Asset Management](#asset-management)
  - [asset create](#asset-create)
  - [asset list](#asset-list)
  - [asset describe](#asset-describe)
  - [asset update](#asset-update)
  - [asset delete](#asset-delete)
- [Portfolio Management](#portfolio-management)
  - [portfolio create](#portfolio-create)
  - [portfolio list](#portfolio-list)
  - [portfolio describe](#portfolio-describe)
  - [portfolio update](#portfolio-update)
  - [portfolio delete](#portfolio-delete)
- [Segment Management](#segment-management)
  - [segment create](#segment-create)
  - [segment list](#segment-list)
  - [segment describe](#segment-describe)
  - [segment update](#segment-update)
  - [segment delete](#segment-delete)
- [Account Management](#account-management)
  - [account create](#account-create)
  - [account list](#account-list)
  - [account describe](#account-describe)
  - [account update](#account-update)
  - [account delete](#account-delete)

## Global Options

These options can be used with any command:

| Option | Description |
|--------|-------------|
| `--no-color` | Disables colored output |
| `-h, --help` | Displays help information about the command |

## Core Commands

### login

Authenticates with the Midaz CLI to access the platform's features.

**Syntax**
```
mdz login [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--username` | Your username |
| `--password` | Your password |
| `-h, --help` | Displays help for the login command |

**Examples**
```bash
# Interactive login (will prompt for method selection)
mdz login

# Direct login with credentials
mdz login --username email@example.com --password Pass@123
```

### configure

Defines the service URLs and credentials for the MDZ CLI environment.

**Syntax**
```
mdz configure [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--client-id` | Unique client identifier used for authentication |
| `--client-secret` | Secret key used to validate the client's identity |
| `--url-api-auth` | URL of the authentication service |
| `--url-api-ledger` | URL of the service responsible for the ledger |
| `-h, --help` | Displays help for the configure command |

**Examples**
```bash
# Interactive configuration (will prompt for values)
mdz configure

# Direct configuration with values
mdz configure --client-id "my-client-id" --client-secret "my-client-secret" --url-api-auth "https://auth.example.com" --url-api-ledger "https://ledger.example.com"
```

### version

Displays the version of the Midaz CLI installed on your system.

**Syntax**
```
mdz version [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `-h, --help` | Displays help for the version command |

**Examples**
```bash
# Display the CLI version
mdz version
```

## Organization Management

The `organization` command manages organizations within the Midaz system, which represent companies that use Midaz, such as banks or other financial institutions.

### organization create

Creates a new organization.

**Syntax**
```
mdz organization create [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--legal-name` | The legal name of the organization |
| `--doing-business-as` | The name under which the organization operates |
| `--legal-document` | Legal document identifier (e.g., tax ID) |
| `--code` | Status code for the organization (e.g., ACTIVE) |
| `--description` | Description of the organization |
| `--line1` | First line of the address |
| `--line2` | Second line of the address |
| `--zip-code` | ZIP/Postal code |
| `--city` | City name |
| `--state` | State or province |
| `--country` | Country code (2-letter ISO code) |
| `--metadata` | Metadata in JSON format |
| `--json-file` | Path to a JSON file containing the organization attributes |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Create an organization with parameters
mdz organization create --legal-name "Acme Corp" --doing-business-as "Acme" --legal-document "12345678901" --code "ACTIVE" --description "Technology company"

# Create from a JSON file
mdz organization create --json-file organization.json
```

### organization list

Lists all organizations the user has access to.

**Syntax**
```
mdz organization list [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# List all organizations
mdz organization list
```

### organization describe

Displays detailed information about a specific organization.

**Syntax**
```
mdz organization describe [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization to describe |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Describe an organization
mdz organization describe --organization-id "123e4567-e89b-12d3-a456-426614174000"
```

### organization update

Updates an existing organization's details.

**Syntax**
```
mdz organization update [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization to update |
| `--legal-name` | The updated legal name |
| `--doing-business-as` | The updated operational name |
| `--legal-document` | Updated legal document identifier |
| `--status-code` | Updated status code |
| `--status-description` | Updated status description |
| `--line1` | Updated first line of address |
| `--line2` | Updated second line of address |
| `--zip-code` | Updated ZIP/Postal code |
| `--city` | Updated city name |
| `--state` | Updated state or province |
| `--country` | Updated country code |
| `--metadata` | Updated metadata in JSON format |
| `--json-file` | Path to a JSON file with update data |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Update an organization
mdz organization update --organization-id "123e4567-e89b-12d3-a456-426614174000" --legal-name "New Acme Corp" --status-code "INACTIVE"
```

### organization delete

Deletes an organization.

**Syntax**
```
mdz organization delete [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization to delete |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Delete an organization
mdz organization delete --organization-id "123e4567-e89b-12d3-a456-426614174000"
```

## Ledger Management

The `ledger` command manages ledgers, which store all the transactions and operations of an organization.

### ledger create

Creates a new ledger within an organization.

**Syntax**
```
mdz ledger create [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--name` | Name of the ledger |
| `--status-code` | Status code for the ledger (e.g., ACTIVE) |
| `--status-description` | Description of the status |
| `--metadata` | Metadata in JSON format |
| `--json-file` | Path to a JSON file containing ledger attributes |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Create a ledger
mdz ledger create --organization-id "123e4567-e89b-12d3-a456-426614174000" --name "Main Ledger" --status-code "ACTIVE"
```

### ledger list

Lists ledgers within an organization.

**Syntax**
```
mdz ledger list [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# List ledgers in an organization
mdz ledger list --organization-id "123e4567-e89b-12d3-a456-426614174000"
```

### ledger describe

Displays detailed information about a specific ledger.

**Syntax**
```
mdz ledger describe [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger to describe |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Describe a ledger
mdz ledger describe --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000"
```

### ledger update

Updates an existing ledger's details.

**Syntax**
```
mdz ledger update [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger to update |
| `--name` | Updated name of the ledger |
| `--status-code` | Updated status code |
| `--status-description` | Updated status description |
| `--metadata` | Updated metadata in JSON format |
| `--json-file` | Path to a JSON file with update data |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Update a ledger
mdz ledger update --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --name "Updated Main Ledger"
```

### ledger delete

Deletes a ledger.

**Syntax**
```
mdz ledger delete [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger to delete |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Delete a ledger
mdz ledger delete --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000"
```

## Asset Management

The `asset` command manages assets within a ledger, representing currencies, commodities, or other financial instruments.

### asset create

Creates a new permitted asset in a ledger.

**Syntax**
```
mdz asset create [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--name` | Name of the asset |
| `--type` | Asset category (e.g., currency, crypto, commodity) |
| `--code` | Asset identifier code (e.g., USD, BTC) |
| `--status-code` | Status code for the asset |
| `--status-description` | Description of the status |
| `--metadata` | Metadata in JSON format |
| `--json-file` | Path to a JSON file containing asset attributes |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Create an asset
mdz asset create --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --name "US Dollar" --code "USD" --type "currency"
```

### asset list

Lists assets within a ledger.

**Syntax**
```
mdz asset list [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# List assets in a ledger
mdz asset list --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000"
```

### asset describe

Displays detailed information about a specific asset.

**Syntax**
```
mdz asset describe [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--asset-id` | The ID of the asset to describe |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Describe an asset
mdz asset describe --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --asset-id "789e0123-e45b-67d8-a901-426614174000"
```

### asset update

Updates an existing asset's details.

**Syntax**
```
mdz asset update [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--asset-id` | The ID of the asset to update |
| `--name` | Updated name of the asset |
| `--status-code` | Updated status code |
| `--status-description` | Updated status description |
| `--metadata` | Updated metadata in JSON format |
| `--json-file` | Path to a JSON file with update data |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Update an asset
mdz asset update --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --asset-id "789e0123-e45b-67d8-a901-426614174000" --name "Updated US Dollar"
```

### asset delete

Deletes an asset.

**Syntax**
```
mdz asset delete [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--asset-id` | The ID of the asset to delete |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Delete an asset
mdz asset delete --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --asset-id "789e0123-e45b-67d8-a901-426614174000"
```

## Portfolio Management

The `portfolio` command manages portfolios, which group related accounts.

### portfolio create

Creates a new portfolio within a ledger.

**Syntax**
```
mdz portfolio create [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--name` | Name of the portfolio |
| `--entity-id` | External entity identifier |
| `--status-code` | Status code for the portfolio |
| `--status-description` | Description of the status |
| `--metadata` | Metadata in JSON format |
| `--json-file` | Path to a JSON file containing portfolio attributes |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Create a portfolio
mdz portfolio create --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --name "Customer Portfolio"
```

### portfolio list

Lists portfolios within a ledger.

**Syntax**
```
mdz portfolio list [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# List portfolios in a ledger
mdz portfolio list --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000"
```

### portfolio describe

Displays detailed information about a specific portfolio.

**Syntax**
```
mdz portfolio describe [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--portfolio-id` | The ID of the portfolio to describe |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Describe a portfolio
mdz portfolio describe --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000"
```

### portfolio update

Updates an existing portfolio's details.

**Syntax**
```
mdz portfolio update [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--portfolio-id` | The ID of the portfolio to update |
| `--name` | Updated name of the portfolio |
| `--entity-id` | Updated external entity identifier |
| `--status-code` | Updated status code |
| `--status-description` | Updated status description |
| `--metadata` | Updated metadata in JSON format |
| `--json-file` | Path to a JSON file with update data |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Update a portfolio
mdz portfolio update --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000" --name "Updated Customer Portfolio"
```

### portfolio delete

Deletes a portfolio.

**Syntax**
```
mdz portfolio delete [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--portfolio-id` | The ID of the portfolio to delete |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Delete a portfolio
mdz portfolio delete --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000"
```

## Segment Management

The `segment` command manages segments, which are used to categorize portfolios and accounts.

### segment create

Creates a new segment within a ledger.

**Syntax**
```
mdz segment create [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--name` | Name of the segment |
| `--status-code` | Status code for the segment |
| `--status-description` | Description of the status |
| `--metadata` | Metadata in JSON format |
| `--json-file` | Path to a JSON file containing segment attributes |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Create a segment
mdz segment create --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --name "Corporate Customers"
```

### segment list

Lists segments within a ledger.

**Syntax**
```
mdz segment list [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# List segments in a ledger
mdz segment list --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000"
```

### segment describe

Displays detailed information about a specific segment.

**Syntax**
```
mdz segment describe [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--segment-id` | The ID of the segment to describe |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Describe a segment
mdz segment describe --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --segment-id "345e6789-e01b-23d4-a567-426614174000"
```

### segment update

Updates an existing segment's details.

**Syntax**
```
mdz segment update [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--segment-id` | The ID of the segment to update |
| `--name` | Updated name of the segment |
| `--status-code` | Updated status code |
| `--status-description` | Updated status description |
| `--metadata` | Updated metadata in JSON format |
| `--json-file` | Path to a JSON file with update data |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Update a segment
mdz segment update --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --segment-id "345e6789-e01b-23d4-a567-426614174000" --name "Updated Corporate Customers"
```

### segment delete

Deletes a segment.

**Syntax**
```
mdz segment delete [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--segment-id` | The ID of the segment to delete |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Delete a segment
mdz segment delete --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --segment-id "345e6789-e01b-23d4-a567-426614174000"
```

## Account Management

The `account` command manages accounts within a portfolio, representing individual financial entities.

### account create

Creates a new account within a portfolio.

**Syntax**
```
mdz account create [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--portfolio-id` | The ID of the portfolio |
| `--segment-id` | The ID of the segment (optional) |
| `--name` | Name of the account |
| `--parent-account-id` | Parent account ID (for sub-accounts) |
| `--alias` | Unique alias for the account |
| `--type` | Account type (e.g., deposit, savings, creditCard) |
| `--asset-code` | Asset code associated with this account (e.g., USD) |
| `--status-code` | Status code for the account (e.g., ACTIVE) |
| `--status-description` | Description of the current status of the account |
| `--entity-id` | The ID of the associated entity (optional) |
| `--metadata` | Metadata in JSON format, e.g., '{"key1": "value", "key2": 123}' |
| `--json-file` | Path to a JSON file containing account attributes, or '-' for stdin |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Create an account
mdz account create --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000" --name "Main Account" --type "deposit" --asset-code "USD"
```

### account list

Lists accounts within a portfolio.

**Syntax**
```
mdz account list [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--portfolio-id` | The ID of the portfolio |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# List accounts in a portfolio
mdz account list --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000"
```

### account describe

Displays detailed information about a specific account.

**Syntax**
```
mdz account describe [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--portfolio-id` | The ID of the portfolio |
| `--account-id` | The ID of the account to describe |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Describe an account
mdz account describe --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000" --account-id "678e9012-e34b-56d7-a890-426614174000"
```

### account update

Updates an existing account's details.

**Syntax**
```
mdz account update [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--portfolio-id` | The ID of the portfolio |
| `--account-id` | The ID of the account to update |
| `--name` | Updated name of the account |
| `--segment-id` | Updated segment ID |
| `--status-code` | Updated status code |
| `--status-description` | Updated status description |
| `--metadata` | Updated metadata in JSON format |
| `--json-file` | Path to a JSON file with update data |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Update an account
mdz account update --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000" --account-id "678e9012-e34b-56d7-a890-426614174000" --name "Updated Main Account"
```

### account delete

Deletes an account.

**Syntax**
```
mdz account delete [flags]
```

**Options**

| Option | Description |
|--------|-------------|
| `--organization-id` | The ID of the organization |
| `--ledger-id` | The ID of the ledger |
| `--portfolio-id` | The ID of the portfolio |
| `--account-id` | The ID of the account to delete |
| `-h, --help` | Displays help for the command |

**Examples**
```bash
# Delete an account
mdz account delete --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000" --account-id "678e9012-e34b-56d7-a890-426614174000"
```