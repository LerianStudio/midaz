# MDZ CLI

**Navigation:** [Home](../../) > [Components](../) > MDZ CLI

## Overview

The MDZ CLI (Command Line Interface) provides a powerful terminal-based interface for interacting with the Midaz ledger system. It allows users to manage financial entities, configure the application, and perform operations against the Midaz API services from the command line.

## Responsibilities

- **Authentication**: Securely authenticate users with the Midaz services
- **Entity Management**: Create, read, update, and delete financial entities
- **Command Execution**: Execute operations against the Midaz API services
- **Configuration**: Manage local configuration and settings
- **Interactive Input**: Provide user-friendly TUI (Terminal User Interface) components
- **Output Formatting**: Present results in a consistent, readable format

## Architecture

The MDZ CLI is built with Go using the Cobra framework for command structure and Charm libraries for interactive TUI components:

```
┌───────────────────────────────────────────────────────┐
│                       MDZ CLI                         │
│                                                       │
│  ┌───────────────┐      ┌───────────────────────┐     │
│  │  Root Command │      │  Factory/Dependency   │     │
│  │  & Bootstrap  │      │      Injection        │     │
│  └───────┬───────┘      └───────────┬───────────┘     │
│          │                          │                 │
│          ▼                          ▼                 │
│  ┌───────────────────────────────────────────────┐    │
│  │               Command Structure               │    │
│  │                                               │    │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐          │    │
│  │  │ Login   │ │ Account │ │ Ledger  │  ...     │    │
│  │  └─────────┘ └─────────┘ └─────────┘          │    │
│  └──────────────────┬────────────────────────────┘    │
│                     │                                  │
│                     ▼                                  │
│  ┌───────────────────────────────────┐                │
│  │        Internal Services          │                │
│  │                                   │                │
│  │  ┌─────────┐ ┌─────────────────┐  │                │
│  │  │ Settings│ │ IO/Output       │  │                │
│  │  └─────────┘ └─────────────────┘  │                │
│  │                                   │                │
│  │  ┌─────────┐ ┌─────────────────┐  │                │
│  │  │ Auth    │ │ REST Clients    │  │                │
│  │  └─────────┘ └─────────────────┘  │                │
│  └───────────────────────────────────┘                │
│                     │                                  │
│                     ▼                                  │
│  ┌───────────────────────────────────┐                │
│  │          TUI Components           │                │
│  │                                   │                │
│  │  ┌─────────┐ ┌─────────────────┐  │                │
│  │  │ Input   │ │ Select          │  │                │
│  │  └─────────┘ └─────────────────┘  │                │
│  │                                   │                │
│  │  ┌─────────┐ ┌─────────────────┐  │                │
│  │  │Password │ │ Output          │  │                │
│  │  └─────────┘ └─────────────────┘  │                │
│  └───────────────────────────────────┘                │
└───────────────────────────────────────────────────────┘
```

## Command Structure

The CLI follows a hierarchical command structure:

- **mdz** - Root command
  - **login** - Authentication
  - **configure** - CLI configuration
  - **organization** - Organization management
    - create, list, describe, update, delete
  - **ledger** - Ledger management
    - create, list, describe, update, delete
  - **asset** - Asset management
    - create, list, describe, update, delete
  - **portfolio** - Portfolio management
    - create, list, describe, update, delete
  - **segment** - Segment management
    - create, list, describe, update, delete
  - **account** - Account management
    - create, list, describe, update, delete
  - **version** - Version information

Each resource management command follows a consistent CRUD pattern.

## Key Features

### Authentication

The CLI supports multiple authentication methods:

- **Browser-based OAuth**: Opens a browser for authentication flow
- **Terminal-based Login**: Username and password entry in the terminal
- **Token Management**: Securely stores and manages authentication tokens

### Interactive Input

When command flags aren't provided, the CLI offers interactive prompts:

- **Text Input**: Gather textual information
- **Password Input**: Securely collect passwords with masked characters
- **Selection Menus**: Choose from available options
- **JSON Input**: Accept file input for complex structures

### Formatted Output

The CLI provides consistent, readable output:

- **Color-coded Output**: Highlight important information
- **Tabular Data**: Display lists in table format
- **JSON Format**: Optional JSON output for scripting

### Configuration Management

Settings are stored locally for persistent configuration:

- **Service URLs**: Connection endpoints for services
- **Authentication**: Token storage and management
- **Output Preferences**: Format and verbosity settings

## Integration Points

The MDZ CLI integrates with other Midaz components:

- **Onboarding Service**: Entity management operations
- **Transaction Service**: Transaction and balance operations
- **Authentication Service**: User authentication and token management

## Usage Examples

### Authentication

```bash
# Interactive login
mdz login

# Login with credentials
mdz login --username user@example.com --password secret

# Browser-based login
mdz login --browser
```

### Entity Management

```bash
# Create an organization
mdz organization create --name "Example Org" --legal-name "Example Inc."

# List ledgers in an organization
mdz ledger list --organization-id org123

# Create an account
mdz account create --ledger-id ldg123 --organization-id org123 --name "Cash Account" --asset-code USD
```

### Configuration

```bash
# Configure CLI settings
mdz configure

# Configure with specific values
mdz configure --api-url https://api.example.com
```

## Next Steps

- [Commands Reference](./commands.md)
- [Usage Guide](./usage-guide.md)