# MDZ CLI Architecture Documentation

## Table of Contents
- [Executive Summary](#executive-summary)
- [System Architecture](#system-architecture)
  - [Component Diagram](#component-diagram)
  - [Data Flow Diagram](#data-flow-diagram)
- [Component Breakdown](#component-breakdown)
  - [Core Components](#core-components)
  - [Command Structure](#command-structure)
  - [Repository Layer](#repository-layer)
  - [REST Adapters](#rest-adapters)
- [Sequence Diagrams](#sequence-diagrams)
  - [Authentication Flow](#authentication-flow)
  - [Command Execution Flow](#command-execution-flow)
  - [CRUD Operation Flow](#crud-operation-flow)
- [API Interfaces](#api-interfaces)
  - [Internal APIs](#internal-apis)
  - [External REST APIs](#external-rest-apis)
- [Data Models](#data-models)
- [Design Decisions](#design-decisions)
- [Notable Implementation Patterns](#notable-implementation-patterns)
- [Potential Improvement Areas](#potential-improvement-areas)

## Executive Summary

MDZ (Midaz CLI) is a command-line interface tool designed to interact with the Midaz ledger system. It follows a clean architecture pattern with clear separation of concerns, leveraging the Factory pattern for dependency injection and the Command pattern via Cobra for CLI structure. The architecture emphasizes testability, extensibility, and maintainability through interface-based design and consistent patterns across all domain entities.

## System Architecture

### Component Diagram

```mermaid
graph TB
    subgraph "MDZ CLI"
        subgraph "Entry Point"
            Main[main.go]
        end
        
        subgraph "Core Layer"
            Factory[Factory<br/>pkg/factory/]
            Env[Environment<br/>pkg/environment/]
            IO[IOStreams<br/>pkg/iostreams/]
            Settings[Settings<br/>pkg/setting/]
        end
        
        subgraph "Command Layer"
            Root[Root Command<br/>pkg/cmd/root/]
            Auth[Auth Commands<br/>pkg/cmd/login/]
            OrgCmd[Organization<br/>pkg/cmd/organization/]
            LedgerCmd[Ledger<br/>pkg/cmd/ledger/]
            AssetCmd[Asset<br/>pkg/cmd/asset/]
            AccountCmd[Account<br/>pkg/cmd/account/]
            TransCmd[Transaction<br/>pkg/cmd/transaction/]
            BalanceCmd[Balance<br/>pkg/cmd/balance/]
            OpCmd[Operation<br/>pkg/cmd/operation/]
        end
        
        subgraph "Domain Layer"
            Repo[Repository Interfaces<br/>internal/domain/repository/]
            Model[Domain Models<br/>internal/model/]
        end
        
        subgraph "Infrastructure Layer"
            REST[REST Adapters<br/>internal/rest/]
            TUI[TUI Components<br/>pkg/tui/]
            Output[Output Formatter<br/>pkg/output/]
        end
    end
    
    subgraph "External Systems"
        API[Midaz Backend APIs]
    end
    
    Main --> Factory
    Factory --> Root
    Factory --> Env
    Factory --> IO
    Factory --> Settings
    
    Root --> Auth
    Root --> OrgCmd
    Root --> LedgerCmd
    Root --> AssetCmd
    Root --> AccountCmd
    Root --> TransCmd
    Root --> BalanceCmd
    Root --> OpCmd
    
    OrgCmd --> Repo
    LedgerCmd --> Repo
    AssetCmd --> Repo
    AccountCmd --> Repo
    TransCmd --> Repo
    BalanceCmd --> Repo
    OpCmd --> Repo
    
    Repo --> REST
    REST --> API
    
    OrgCmd --> TUI
    OrgCmd --> Output
```

### Data Flow Diagram

```mermaid
flowchart LR
    subgraph "User Input"
        CLI[CLI Command]
        Flags[Flags/Args]
        Config[Config Files]
    end
    
    subgraph "MDZ CLI Processing"
        Parse[Command Parser]
        Validate[Input Validation]
        Factory[Factory Creation]
        Execute[Command Execution]
        Format[Output Formatting]
    end
    
    subgraph "External Communication"
        HTTP[HTTP Client]
        API[Backend API]
    end
    
    subgraph "Output"
        Terminal[Terminal Output]
        JSON[JSON Output]
        Table[Table Output]
    end
    
    CLI --> Parse
    Flags --> Parse
    Config --> Factory
    
    Parse --> Validate
    Validate --> Factory
    Factory --> Execute
    Execute --> HTTP
    HTTP --> API
    API --> HTTP
    HTTP --> Execute
    Execute --> Format
    
    Format --> Terminal
    Format --> JSON
    Format --> Table
```

## Component Breakdown

### Core Components

#### 1. Main Entry Point (`main.go`)
- **Purpose**: Bootstrap the application
- **Responsibilities**:
  - Initialize environment configuration
  - Create factory instance
  - Execute root command
  - Handle fatal errors

#### 2. Factory (`pkg/factory/factory.go`)
- **Purpose**: Central dependency container
- **Key Components**:
  ```go
  type Factory struct {
      HTTPClient  *http.Client
      IOStreams   iostreams.IOStreams
      Config      *Config
      Environment environment.Env
      Token       string
  }
  ```
- **Responsibilities**:
  - Provide dependencies to all commands
  - Manage HTTP client configuration
  - Handle authentication token

#### 3. Environment (`pkg/environment/environment.go`)
- **Purpose**: Manage build-time and runtime configuration
- **Key Fields**:
  - `Version`: CLI version
  - `GitCommit`: Build commit hash
  - `Date`: Build date
  - `APIAddress`: Backend API URL
  - `AuthURL`: Authentication service URL

#### 4. IOStreams (`pkg/iostreams/iostreams.go`)
- **Purpose**: Abstract input/output operations
- **Components**:
  - `In`: Input reader
  - `Out`: Output writer
  - `Err`: Error writer
  - `EnableColor`: Color output control

### Command Structure

Each domain entity follows a consistent command pattern:

#### Standard Commands per Entity:
1. **Create** - Create new resource
2. **List** - List all resources
3. **Describe** - Get detailed information
4. **Update** - Update existing resource
5. **Delete** - Remove resource

#### Command Organization (`pkg/cmd/`):
```
cmd/
├── root/          # Root command orchestration
├── login/         # Authentication commands
├── configure/     # Configuration management
├── organization/  # Organization CRUD
├── ledger/        # Ledger CRUD
├── asset/         # Asset CRUD
├── portfolio/     # Portfolio CRUD
├── segment/       # Segment CRUD
├── account/       # Account CRUD
├── transaction/   # Transaction operations
├── operation/     # Operation queries
├── balance/       # Balance queries
├── asset_rate/    # Asset rate management
└── version/       # Version information
```

### Repository Layer

#### Repository Interfaces (`internal/domain/repository/`)
Each domain entity has a repository interface defining the contract:

```go
// Example: Account Repository
type Account interface {
    Create(context.Context, string, string, CreateAccountInput) (*mmodel.Account, error)
    Get(context.Context, string, string, QueryPagination) (*mmodel.Accounts, error)
    GetByID(context.Context, string, string, string) (*mmodel.Account, error)
    Update(context.Context, string, string, string, UpdateAccountInput) (*mmodel.Account, error)
    Delete(context.Context, string, string, string) error
}
```

### REST Adapters

#### REST Implementation (`internal/rest/`)
- Implements repository interfaces
- Handles HTTP communication
- Manages request/response serialization
- Error handling and status code mapping

## Sequence Diagrams

### Authentication Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant LoginCmd
    participant Settings
    participant AuthAPI
    participant Browser
    
    User->>CLI: mdz login
    CLI->>LoginCmd: Execute()
    
    alt Browser Mode
        LoginCmd->>Browser: Open Auth URL
        Browser->>AuthAPI: OAuth Flow
        AuthAPI-->>Browser: Auth Code
        Browser-->>LoginCmd: Callback with Code
    else Terminal Mode
        LoginCmd->>User: Enter credentials
        User->>LoginCmd: Username/Password
        LoginCmd->>AuthAPI: Authenticate
    end
    
    AuthAPI-->>LoginCmd: Access Token
    LoginCmd->>Settings: Save Token
    Settings-->>LoginCmd: Success
    LoginCmd-->>User: Login successful
```

### Command Execution Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Root
    participant Command
    participant Factory
    participant Repository
    participant REST
    participant API
    
    User->>CLI: mdz <entity> <action> [flags]
    CLI->>Root: Parse command
    Root->>Factory: Create factory
    Factory-->>Root: Factory instance
    Root->>Command: Execute with factory
    
    Command->>Command: Validate inputs
    Command->>Factory: Get repository
    Factory->>REST: Create REST client
    REST-->>Command: Repository implementation
    
    Command->>Repository: Call method
    Repository->>REST: Transform to HTTP
    REST->>API: HTTP Request
    API-->>REST: HTTP Response
    REST->>Repository: Parse response
    Repository-->>Command: Domain model
    
    Command->>Command: Format output
    Command-->>User: Display result
```

### CRUD Operation Flow

```mermaid
sequenceDiagram
    participant User
    participant CreateCmd
    participant TUI
    participant Validator
    participant Repository
    participant API
    participant Output
    
    User->>CreateCmd: mdz account create
    
    alt Missing Required Fields
        CreateCmd->>TUI: Prompt for input
        TUI->>User: Enter field value
        User->>TUI: Input value
        TUI-->>CreateCmd: Field value
    end
    
    CreateCmd->>Validator: Validate inputs
    Validator-->>CreateCmd: Validation result
    
    alt Validation Failed
        CreateCmd-->>User: Error message
    else Validation Passed
        CreateCmd->>Repository: Create(input)
        Repository->>API: POST /accounts
        API-->>Repository: 201 Created
        Repository-->>CreateCmd: Account model
        CreateCmd->>Output: Format result
        Output-->>User: Display created account
    end
```

## API Interfaces

### Internal APIs

#### Repository Interfaces
All repository interfaces follow a consistent pattern located in `internal/domain/repository/`:

1. **Organization** (`organization.go`)
   - `Create()`, `Get()`, `GetByID()`, `Update()`, `Delete()`

2. **Ledger** (`ledger.go`)
   - `Create()`, `Get()`, `GetByID()`, `Update()`, `Delete()`

3. **Asset** (`asset.go`)
   - `Create()`, `Get()`, `GetByID()`, `Update()`, `Delete()`

4. **Account** (`account.go`)
   - `Create()`, `Get()`, `GetByID()`, `Update()`, `Delete()`

5. **Transaction** (`transaction.go`)
   - `Create()`, `CreateFromDSL()`, `Get()`, `GetByID()`, `Revert()`

6. **Balance** (`balance.go`)
   - `Get()`, `GetByID()`, `GetByAccount()`, `Delete()`

7. **Operation** (`operation.go`)
   - `Get()`, `GetByID()`, `GetByAccount()`

### External REST APIs

The CLI communicates with the following backend API endpoints:

#### Authentication Endpoints
- `POST /v1/auth/authenticate` - Authenticate user
- `POST /v1/auth/token` - Exchange auth code for token

#### Organization Endpoints
- `GET /v1/organizations` - List organizations
- `POST /v1/organizations` - Create organization
- `GET /v1/organizations/{id}` - Get organization by ID
- `PATCH /v1/organizations/{id}` - Update organization
- `DELETE /v1/organizations/{id}` - Delete organization

#### Ledger Endpoints
- `GET /v1/organizations/{orgId}/ledgers` - List ledgers
- `POST /v1/organizations/{orgId}/ledgers` - Create ledger
- `GET /v1/organizations/{orgId}/ledgers/{id}` - Get ledger
- `PATCH /v1/organizations/{orgId}/ledgers/{id}` - Update ledger
- `DELETE /v1/organizations/{orgId}/ledgers/{id}` - Delete ledger

#### Asset Endpoints
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/assets` - List assets
- `POST /v1/organizations/{orgId}/ledgers/{ledgerId}/assets` - Create asset
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/assets/{id}` - Get asset
- `PATCH /v1/organizations/{orgId}/ledgers/{ledgerId}/assets/{id}` - Update asset
- `DELETE /v1/organizations/{orgId}/ledgers/{ledgerId}/assets/{id}` - Delete asset

#### Account Endpoints
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/accounts` - List accounts
- `POST /v1/organizations/{orgId}/ledgers/{ledgerId}/accounts` - Create account
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/accounts/{id}` - Get account
- `PATCH /v1/organizations/{orgId}/ledgers/{ledgerId}/accounts/{id}` - Update account
- `DELETE /v1/organizations/{orgId}/ledgers/{ledgerId}/accounts/{id}` - Delete account

#### Transaction Endpoints
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/transactions` - List transactions
- `POST /v1/organizations/{orgId}/ledgers/{ledgerId}/transactions` - Create transaction
- `POST /v1/organizations/{orgId}/ledgers/{ledgerId}/transactions/dsl` - Create from DSL
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/transactions/{id}` - Get transaction
- `POST /v1/organizations/{orgId}/ledgers/{ledgerId}/transactions/{id}/revert` - Revert transaction

#### Balance Endpoints
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/balances` - List balances
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/balances/{id}` - Get balance
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/accounts/{accountId}/balances` - Get account balances
- `DELETE /v1/organizations/{orgId}/ledgers/{ledgerId}/balances/{id}` - Delete balance

#### Operation Endpoints
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/operations` - List operations
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/operations/{id}` - Get operation
- `GET /v1/organizations/{orgId}/ledgers/{ledgerId}/accounts/{accountId}/operations` - Get account operations

## Data Models

The CLI uses models from the shared `pkg/mmodel` package. Key models include:

### Core Entities
- **Organization**: Root entity for multi-tenancy
- **Ledger**: Container for accounts and transactions
- **Asset**: Currency or asset definitions
- **Portfolio**: Groups of accounts
- **Segment**: Account categorization
- **Account**: Individual account in the ledger
- **Transaction**: Double-entry bookkeeping transaction
- **Operation**: Individual debit/credit operation
- **Balance**: Account balance snapshot

### Authentication Models
- **Credentials**: Username/password for authentication
- **Token**: Access token response

## Design Decisions

### 1. Factory Pattern for Dependency Injection
- **Rationale**: Provides clean dependency management without global state
- **Benefits**: Testability, flexibility, clear dependencies

### 2. Interface-Based Repository Pattern
- **Rationale**: Decouple business logic from infrastructure
- **Benefits**: Easy mocking, swappable implementations

### 3. Cobra Command Framework
- **Rationale**: Industry-standard CLI framework
- **Benefits**: Consistent UX, built-in help, flag parsing

### 4. Structured Error Handling
- **Rationale**: Consistent error reporting across commands
- **Benefits**: Better debugging, user-friendly messages

### 5. TUI for Interactive Input
- **Rationale**: Guide users through complex inputs
- **Benefits**: Better UX for missing required fields

## Notable Implementation Patterns

### 1. Command Factory Pattern
Each command has its own factory struct that encapsulates:
- Repository dependencies
- Flags/configuration
- Execution logic

Example:
```go
type factoryAccountCreate struct {
    factory      *factory.Factory
    repoAccount  repository.Account
    tuiInput     func(string) (string, error)
    flagsCreate  // Embedded flags struct
}
```

### 2. Consistent Flag Structure
All CRUD commands follow the same flag pattern:
- Required: `--organization-id`, `--ledger-id`
- Optional: `--output` (json/table)
- Entity-specific flags

### 3. Output Formatting
Flexible output system supporting:
- Table format (default)
- JSON format (for scripting)
- Consistent formatting across all commands

### 4. Mock Generation
Comprehensive mocking support:
- Repository interfaces have corresponding mocks
- Used extensively in unit tests
- Generated with mockery tool

### 5. Golden File Testing
Output validation using golden files:
- Captures expected output
- Ensures consistency across changes
- Located in `testdata/` directories

## Recent Improvements

### 1. Enhanced Error Recovery (✅ Implemented)
- **Retry Logic**: Automatic retry with exponential backoff for transient failures
- **Better Error Messages**: Context-aware error messages with helpful suggestions
- **Rollback Support**: Transaction-style operations with automatic rollback on failure
- **Location**: `pkg/errors/` - Comprehensive error handling package

### 2. Interactive Mode (✅ Implemented)
- **REPL Interface**: Full interactive mode with `mdz interactive` command
- **Command History**: Persistent command history with readline support
- **Auto-completion**: Tab completion for commands, subcommands, and flags
- **Built-in Commands**: Special REPL commands (history, clear, pwd)
- **Location**: `pkg/repl/`, `pkg/cmd/interactive/`

### 3. Audit Trail (✅ Implemented)
- **Command History**: All commands logged with timestamp, duration, and result
- **Operation Logging**: Detailed audit trail stored in `~/.mdz/audit.json`
- **Undo/Redo**: Support for undoing create operations with `mdz undo`
- **History Viewer**: `mdz history` command to view and manage audit trail
- **Location**: `pkg/audit/`, `pkg/cmd/history/`, `pkg/cmd/undo/`

### 4. Type Safety Improvements (✅ Implemented)
- **Modern Go**: Replaced `interface{}` with `any` throughout the codebase
- **Enhanced Error Types**: Strongly typed error system with rich context
- **Type-safe Builders**: Fluent interfaces for building complex objects

## Remaining Improvement Areas

### 1. Offline Mode
- Cache frequently accessed data
- Queue operations for later sync
- Better handling of network failures

### 2. Plugin System
- Allow custom commands
- Extension points for enterprise features
- Dynamic command loading

### 3. Configuration Management
- Support for configuration profiles
- Environment-specific settings
- Secure credential storage

### 4. Performance Optimizations
- Parallel operations where applicable
- Response caching
- Lazy loading of command dependencies

### 5. Batch Operations
- Support for bulk operations
- Transaction batching
- CSV import/export

### 6. Observability
- Command execution metrics
- Performance profiling
- Debug mode with detailed logging

### 7. Advanced Undo/Redo
- State snapshots for update operations
- Undo stack with multiple levels
- Selective undo of specific operations

### 8. Command Aliases
- User-defined command shortcuts
- Persistent alias storage
- Alias expansion in interactive mode