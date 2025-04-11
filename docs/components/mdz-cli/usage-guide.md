# Usage Guide

**Navigation:** [Home](../../) > [MDZ CLI](../) > Usage Guide

This guide provides comprehensive instructions for installing, configuring, and using the Midaz CLI (`mdz`). It covers basic operations, advanced features, common workflows, and best practices.

## Table of Contents

- [Introduction](#introduction)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [Authentication](#authentication)
- [Basic Usage Patterns](#basic-usage-patterns)
- [Entity Management](#entity-management)
- [Advanced Features](#advanced-features)
- [Common Workflows](#common-workflows)
- [Output Formatting](#output-formatting)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

## Introduction

The MDZ CLI (Command Line Interface) is a powerful tool that allows you to interact with the Midaz financial platform directly from your terminal. It provides a comprehensive set of commands for managing all aspects of your financial entities, from organizations to accounts, and supports both interactive and programmatic usage.

## Installation

### macOS/Linux

```bash
# Download the latest release (replace VERSION with the actual version)
curl -L https://github.com/LerianStudio/midaz/releases/download/vVERSION/mdz-cli-VERSION-darwin-amd64.tar.gz -o mdz.tar.gz

# Extract and install
tar -xzf mdz.tar.gz
sudo mv mdz /usr/local/bin/
chmod +x /usr/local/bin/mdz
```

### Windows

```powershell
# Using Chocolatey
choco install mdz
```

### Verify Installation

To verify that the installation was successful:

```bash
mdz version
```

This should display the current version of the Midaz CLI.

## Getting Started

After installation, the first steps are to:

1. Configure the CLI to connect to the Midaz services
2. Log in to authenticate your session

### First-time Setup

```bash
# Configure services
mdz configure

# Log in to the platform
mdz login
```

## Configuration

The `configure` command allows you to set up your CLI environment. Configuration is stored in `~/.config/mdz/mdz.toml`.

### Interactive Configuration

Run the following command and follow the prompts:

```bash
mdz configure
```

The CLI will interactively ask for:
- API endpoint URLs
- Client credentials (if applicable)
- Output preferences

### Direct Configuration

Alternatively, you can specify configuration parameters directly:

```bash
mdz configure --client-id "my-client-id" --client-secret "my-client-secret" --url-api-auth "https://auth.example.com" --url-api-ledger "https://ledger.example.com"
```

### Configuration Options

| Setting | Description | Example |
|---------|-------------|---------|
| `url-api-auth` | Authentication service URL | https://auth.midaz.example.com |
| `url-api-ledger` | Ledger service URL | https://ledger.midaz.example.com |
| `client-id` | Client identifier | my-client-id |
| `client-secret` | Client secret | my-secret-key |

### Viewing Current Configuration

To view your current configuration:

```bash
mdz configure --show
```

## Authentication

The `login` command provides authentication with the Midaz platform. The CLI supports multiple authentication methods:

### Browser-based Authentication

```bash
mdz login --browser
```

This opens your default web browser to complete the authentication process, which is useful for OAuth flows.

### Terminal-based Authentication

```bash
mdz login
```

Without parameters, the CLI will prompt you for credentials interactively with password masking.

### Non-interactive Authentication

```bash
mdz login --username "user@example.com" --password "your-password"
```

This method is useful for scripting but requires caution with password handling.

### Token Management

After successful authentication, the CLI stores the authentication token locally. The token is automatically refreshed when needed. You can force a refresh with:

```bash
mdz login --refresh
```

## Basic Usage Patterns

The Midaz CLI follows consistent patterns across all commands:

### Getting Help

For any command, you can add `--help` or `-h` to see available options:

```bash
# General help
mdz --help

# Command-specific help
mdz organization create --help
```

### Command Structure

Commands are organized hierarchically:
- `mdz <resource> <action>` (e.g., `mdz organization create`)
- Each resource type (organization, ledger, etc.) has consistent actions (create, list, describe, update, delete)

### Interactive vs. Flag-based Usage

Most commands support both:
- **Interactive mode**: Run without all required flags to enter interactive prompts
- **Flag-based mode**: Provide all required parameters as flags

Example:
```bash
# Interactive
mdz organization create

# Flag-based
mdz organization create --legal-name "Acme Corp" --doing-business-as "Acme"
```

### Using JSON Files for Input

For complex entity creation, you can use JSON files:

```bash
mdz organization create --json-file organization.json
```

### Global Options

| Option | Description |
|--------|-------------|
| `--no-color` | Disables colored output |
| `--help` | Displays help information |

## Entity Management

The Midaz CLI provides comprehensive management for all entity types in the financial hierarchy.

### Organization Management

Organizations are the top-level entities in Midaz.

```bash
# Create an organization
mdz organization create --legal-name "Acme Corp" --doing-business-as "Acme" --legal-document "12345678901"

# List all organizations
mdz organization list

# Get details of a specific organization
mdz organization describe --organization-id "123e4567-e89b-12d3-a456-426614174000"

# Update an organization
mdz organization update --organization-id "123e4567-e89b-12d3-a456-426614174000" --legal-name "Acme Corporation"

# Delete an organization
mdz organization delete --organization-id "123e4567-e89b-12d3-a456-426614174000"
```

### Ledger Management

Ledgers store all the financial records for an organization.

```bash
# Create a ledger
mdz ledger create --organization-id "123e4567-e89b-12d3-a456-426614174000" --name "Main Ledger"

# List ledgers in an organization
mdz ledger list --organization-id "123e4567-e89b-12d3-a456-426614174000"

# Get ledger details
mdz ledger describe --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000"
```

### Asset Management

Assets represent currencies or financial instruments in a ledger.

```bash
# Create an asset
mdz asset create --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --name "US Dollar" --code "USD" --type "currency"

# List assets
mdz asset list --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000"
```

### Portfolio and Account Management

Portfolios group related accounts, while accounts represent individual financial holdings.

```bash
# Create a portfolio
mdz portfolio create --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --name "Customer Portfolio"

# Create an account in a portfolio
mdz account create --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000" --name "Main Account" --type "deposit" --asset-code "USD"

# List accounts
mdz account list --organization-id "123e4567-e89b-12d3-a456-426614174000" --ledger-id "456e7890-e12b-34d5-a678-426614174000" --portfolio-id "012e3456-e78b-90d1-a234-426614174000"
```

## Advanced Features

### Using Metadata

Most entity creation commands support attaching metadata:

```bash
mdz account create --name "Savings Account" --metadata '{"purpose": "vacation", "tax_category": "personal"}'
```

### Environment Variables

Critical parameters can be provided via environment variables:

```bash
# Set environment variables
export MDZ_ORGANIZATION_ID="123e4567-e89b-12d3-a456-426614174000"
export MDZ_LEDGER_ID="456e7890-e12b-34d5-a678-426614174000"

# Use commands without specifying these IDs
mdz asset list
```

### Piping and Redirection

You can pipe JSON input or redirect output:

```bash
# Pipe input from another command
cat account.json | mdz account create --json-file -

# Redirect output to a file
mdz organization list > organizations.txt
```

## Common Workflows

### Setting Up a New Organization

```bash
# Create an organization
mdz organization create --legal-name "Example Corp" --doing-business-as "Example"

# Capture the organization ID
ORGANIZATION_ID=$(mdz organization list --output json | jq -r '.items[0].id')

# Create a ledger in the organization
mdz ledger create --organization-id "$ORGANIZATION_ID" --name "Main Ledger"

# Capture the ledger ID
LEDGER_ID=$(mdz ledger list --organization-id "$ORGANIZATION_ID" --output json | jq -r '.items[0].id')

# Set up assets in the ledger
mdz asset create --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --name "US Dollar" --code "USD" --type "currency"
```

### Managing Customer Accounts

```bash
# Create a customer portfolio
mdz portfolio create --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --name "Customer 1" --entity-id "CUST001"

# Capture the portfolio ID
PORTFOLIO_ID=$(mdz portfolio list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --output json | jq -r '.items[0].id')

# Create customer accounts
mdz account create --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --portfolio-id "$PORTFOLIO_ID" --name "Checking" --type "deposit" --asset-code "USD"
mdz account create --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --portfolio-id "$PORTFOLIO_ID" --name "Savings" --type "savings" --asset-code "USD"
```

## Output Formatting

The CLI provides several output formats:

### Default (Human-readable)

```bash
mdz organization list
```

Output is presented in a formatted table with colored highlights.

### JSON Output

For programmatic usage:

```bash
mdz organization list --output json
```

### Controlling Color

By default, output is colored. To disable:

```bash
mdz organization list --no-color
```

## Troubleshooting

### Common Issues

#### Authentication Errors

If you encounter authentication errors:

```bash
# Refresh your token
mdz login --refresh

# If that doesn't work, reconfigure and log in again
mdz configure
mdz login
```

#### Connection Issues

If you have connection problems:

```bash
# Verify your configuration
mdz configure --show

# Test connectivity
mdz version
```

#### Command Errors

If a command fails:

1. Check the error message for details
2. Ensure all required parameters are provided
3. Verify entity IDs exist and are correctly formatted
4. Use `--help` to see the command's requirements

### Getting Help

For detailed information about a specific command:

```bash
mdz <command> --help
```

## Best Practices

### Security

- Avoid using `--password` in scripts or command history
- Use browser-based authentication when possible
- Protect your configuration file (`~/.config/mdz/mdz.toml`)

### Efficiency

- Use JSON files for complex entity creation
- Leverage environment variables for frequently used IDs
- Create shell aliases for common command patterns

### Scripting

- Use JSON output (`--output json`) for programmatic parsing
- Consider error handling in scripts
- Validate entity existence before operations
- Store IDs in variables for reuse

### General Usage

- Start with interactive mode to learn the command flow
- Move to flag-based commands for efficiency
- Group related operations in scripts or aliases
- Keep your CLI version up-to-date