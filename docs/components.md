# Midaz System Components

This document provides a detailed overview of the main components in the Midaz system, explaining their purpose, structure, and how they interact with each other.

## Table of Contents

1. [Infra Component](#infra-component)
2. [MDZ Component (CLI)](#mdz-component-cli)
3. [Onboarding Component](#onboarding-component)
4. [Transaction Component](#transaction-component)
5. [Shared Packages](#shared-packages)

## Infra Component

The Infra component provides all the necessary infrastructure services for running the Midaz system. It's located in the `components/infra` directory.

### Purpose

- Provide database services (PostgreSQL, MongoDB)
- Provide message queue service (RabbitMQ)
- Provide caching service (Redis)
- Provide monitoring and observability (OpenTelemetry)

### Key Files and Directories

- `docker-compose.yml`: Defines all infrastructure services
- `.env.example`: Example environment variables for configuration
- `postgres/`: PostgreSQL configuration files
- `mongo/`: MongoDB configuration files
- `rabbitmq/`: RabbitMQ configuration files
- `grafana/`: Grafana configuration files

### Services

#### PostgreSQL

PostgreSQL is used as the primary database for storing structured data:

```yaml
midaz-postgres-primary:
  container_name: midaz-postgres-primary
  image: postgres:latest
  restart: always
  user: ${USER_EXECUTE_COMMAND}
  healthcheck:
    test: [ "CMD-SHELL", "pg_isready -U ${DB_USER} -p ${DB_PORT}" ]
    interval: 10s
    timeout: 5s
    retries: 5
  ports:
    - ${DB_PORT}:${DB_PORT}
  environment:
    PGPORT: ${DB_PORT}
    POSTGRES_USER: ${DB_USER}
    POSTGRES_PASSWORD: ${DB_PASSWORD}
    POSTGRES_HOST_AUTH_METHOD: "scram-sha-256\nhost replication all 0.0.0.0/0 md5"
    POSTGRES_INITDB_ARGS: "--auth-host=scram-sha-256"
  command: |
    postgres
    -c wal_level=logical
    -c hot_standby=on
    -c max_wal_senders=10
    -c max_replication_slots=10
    -c hot_standby_feedback=on
    -c max_connections=${MAX_CONNECTIONS}
    -c shared_buffers=${SHARED_BUFFERS}
  volumes:
    - postgres-data:/var/lib/postgresql/data
    - ./postgres/init.sql:/docker-entrypoint-initdb.d/init.sql
  networks:
    - infra-network
```

A replica is also configured for high availability:

```yaml
midaz-postgres-replica:
  container_name: midaz-postgres-replica
  image: postgres:latest
  restart: always
  user: ${USER_EXECUTE_COMMAND}
  ports:
    - ${DB_REPLICA_PORT}:${DB_REPLICA_PORT}
  environment:
    PGPORT: ${DB_REPLICA_PORT}
    PGUSER: ${REPLICATION_USER}
    PGPASSWORD: ${REPLICATION_PASSWORD}
  command: |
    bash -c "
    if [ ! -d \"/var/lib/postgresql/data\" ] || [ ! -f \"/var/lib/postgresql/data/postgresql.conf\" ]; then
      until pg_basebackup --pgdata=/var/lib/postgresql/data -R --slot=replication_slot --host=midaz-postgres-primary --port=${DB_PORT}
      do
        echo 'Waiting for midaz-postgres-primary to connect...'
        sleep 1s
      done
    
      echo 'Backup done..., starting midaz-postgres-replica...'
      chmod 0700 /var/lib/postgresql/data
    
      # Ensure the port is set to use for the replica
      sed -i 's/^#port.*/port = ${DB_REPLICA_PORT}/' /var/lib/postgresql/data/postgresql.conf
    
      # Define database max conn
      sed -i 's/^#*max_connections.*/max_connections = ${MAX_CONNECTIONS}/' /var/lib/postgresql/data/postgresql.conf      
    
      # Define database shared buffers
      sed -i 's/^#*shared_buffers.*/shared_buffers = ${SHARED_BUFFERS}/' /var/lib/postgresql/data/postgresql.conf      
    fi
    exec postgres -c config_file=/var/lib/postgresql/data/postgresql.conf
    "
  healthcheck:
    test: [ "CMD-SHELL", "pg_isready -U ${DB_REPLICA_USER} -p ${DB_REPLICA_PORT}" ]
    interval: 10s
    timeout: 5s
    retries: 5
  depends_on:
    midaz-postgres-primary:
      condition: service_healthy
  networks:
    - infra-network
```

#### MongoDB

MongoDB is used for storing document-based data:

```yaml
midaz-mongodb:
  container_name: midaz-mongodb
  image: mongo:latest
  env_file:
    - .env
  restart: always
  healthcheck:
    test: echo 'db.runCommand("ping").ok'
    interval: 10s
    timeout: 5s
    retries: 5
  environment:
    MONGO_INITDB_ROOT_USERNAME: ${MONGO_USER}
    MONGO_INITDB_ROOT_PASSWORD: ${MONGO_PASSWORD}
  ports:
    - ${MONGO_PORT}:${MONGO_PORT}
  command: [ "mongod", "--replSet", "rs0", "--bind_ip_all", "--keyFile", "/etc/mongo-keyfile", "--auth", "--port", "${MONGO_PORT}" ]
  volumes:
    - mongodb-data:/data/db
    - ./mongo/mongo-keyfile:/etc/mongo-keyfile:ro
  networks:
    - infra-network
```

#### Redis

Redis is used for caching and session management:

```yaml
midaz-redis:
  container_name: midaz-redis
  image: valkey/valkey:latest
  restart: always
  env_file:
    - .env
  environment:
    - REDIS_USER=${REDIS_USER}
    - REDIS_PASSWORD=${REDIS_PASSWORD}
  command: ["redis-server", "--requirepass", "${REDIS_PASSWORD}",  "--port", "${REDIS_PORT}"]
  ports:
    - ${REDIS_PORT}:${REDIS_PORT}
  volumes:
    - redis-data:/data
  networks:
    - infra-network
```

#### RabbitMQ

RabbitMQ is used for message queuing:

```yaml
midaz-rabbitmq:
  image: rabbitmq:4.0-management-alpine
  container_name: midaz-rabbitmq
  restart: always
  environment:
    RABBITMQ_DEFAULT_USER: ${RABBITMQ_DEFAULT_USER}
    RABBITMQ_DEFAULT_PASS: ${RABBITMQ_DEFAULT_PASS}
  ports:
    - ${RABBITMQ_PORT_HOST}:${RABBITMQ_PORT_HOST}
    - ${RABBITMQ_PORT_AMPQ}:${RABBITMQ_PORT_AMPQ}
  volumes:
    - ./rabbitmq/etc/definitions.json:/etc/rabbitmq/definitions.json
    - ./rabbitmq/etc/rabbitmq.conf:/etc/rabbitmq/rabbitmq.conf
    - ~/.docker-conf/rabbitmq/data/:/var/lib/rabbitmq/
    - ~/.docker-conf/rabbitmq/log/:/var/log/rabbitmq
  networks:
    - infra-network
```

#### OpenTelemetry

OpenTelemetry is used for monitoring and observability:

```yaml
midaz-otel-lgtm:
  container_name: midaz-otel-lgtm
  image: grafana/otel-lgtm:latest
  restart: always
  environment:
    GF_SECURITY_ADMIN_USER: ${OTEL_LGTM_ADMIN_USER}
    GF_SECURITY_ADMIN_PASSWORD: ${OTEL_LGTM_ADMIN_PASSWORD}
  ports:
    - ${OTEL_LGTM_EXTERNAL_PORT}:${OTEL_LGTM_INTERNAL_PORT}
    - ${OTEL_LGTM_RECEIVER_GRPC_PORT}:${OTEL_LGTM_RECEIVER_GRPC_PORT}
    - ${OTEL_LGTM_RECEIVER_HTTP_PORT}:${OTEL_LGTM_RECEIVER_HTTP_PORT}
  volumes:
    - grafana-data:/otel-lgtm/grafana/data
    - ./grafana/run-grafana.sh:/otel-lgtm/run-grafana.sh
  networks:
    - infra-network
```

## MDZ Component (CLI)

The MDZ component is a command-line interface for interacting with the Midaz system. It's located in the `components/mdz` directory.

### Purpose

- Provide a command-line interface for managing all aspects of the Midaz system
- Allow developers and administrators to interact with the system programmatically
- Support automation through scripting

### Key Files and Directories

- `main.go`: Entry point for the CLI
- `pkg/cmd/`: Command implementations
- `pkg/environment/`: Environment configuration
- `pkg/factory/`: Factory for creating command instances
- `pkg/iostreams/`: I/O streams for command input/output
- `pkg/output/`: Output formatting
- `pkg/setting/`: Settings management

### Command Structure

The MDZ CLI follows a hierarchical command structure:

```go
// Root command
func NewCmdRoot(f *factory.Factory) *cobra.Command {
    // ...
    cmd.AddCommand(version.NewCmdVersion(f.factory))
    cmd.AddCommand(login.NewCmdLogin(f.factory))
    cmd.AddCommand(organization.NewCmdOrganization(f.factory))
    cmd.AddCommand(ledger.NewCmdLedger(f.factory))
    cmd.AddCommand(asset.NewCmdAsset(f.factory))
    cmd.AddCommand(portfolio.NewCmdPortfolio(f.factory))
    cmd.AddCommand(segment.NewCmdSegment(f.factory))
    cmd.AddCommand(account.NewCmdAccount(f.factory))
    cmd.AddCommand(configure.NewCmdConfigure(configure.NewInjectFacConfigure(f.factory)))
    // ...
}
```

Each command has subcommands for specific operations:

```
mdz
├── version
├── login
├── organization
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
├── ledger
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
// ...
```

### Authentication

The MDZ CLI uses token-based authentication:

```go
func (f *factoryRoot) persistentPreRunE(cmd *cobra.Command, _ []string) error {
    // ...
    sett, err := setting.Read()
    if err != nil {
        return errors.New("Try the login command first 'mdz login -h' " + err.Error())
    }

    if len(sett.Env.ClientID) > 0 {
        f.factory.Env.ClientID = sett.ClientID
    }

    if len(sett.Env.ClientSecret) > 0 {
        f.factory.Env.ClientSecret = sett.ClientSecret
    }

    if len(sett.Env.URLAPIAuth) > 0 {
        f.factory.Env.URLAPIAuth = sett.URLAPIAuth
    }

    if len(sett.Env.URLAPILedger) > 0 {
        f.factory.Env.URLAPILedger = sett.URLAPILedger
    }

    if len(sett.Token) > 0 {
        f.factory.Token = sett.Token
    }
    // ...
}
```

## Onboarding Component

The Onboarding component is the core service for managing the fundamental entities in the Midaz system. It's located in the `components/onboarding` directory.

### Purpose

- Manage organizations, ledgers, accounts, assets, portfolios, and segments
- Provide a RESTful API for CRUD operations on these entities
- Ensure data integrity and consistency

### Key Files and Directories

- `api/`: API definitions and documentation
- `cmd/app/`: Application entry point
- `internal/adapters/`: Interface adapters
- `internal/bootstrap/`: Application bootstrapping
- `internal/services/`: Business logic services
- `migrations/`: Database migration scripts

### Architecture

The Onboarding service follows a clean architecture pattern with clear separation of concerns:

```
onboarding/
├── api/                  # API definitions and documentation
├── cmd/                  # Application entry points
│   └── app/              # Main application
├── internal/             # Internal packages
│   ├── adapters/         # Interface adapters
│   │   ├── http/         # HTTP adapters
│   │   │   ├── in/       # Inbound HTTP adapters
│   │   │   └── out/      # Outbound HTTP adapters
│   │   ├── mongodb/      # MongoDB adapters
│   │   ├── postgres/     # PostgreSQL adapters
│   │   │   ├── account/  # Account repository
│   │   │   ├── asset/    # Asset repository
│   │   │   ├── ledger/   # Ledger repository
│   │   │   ├── organization/ # Organization repository
│   │   │   ├── portfolio/    # Portfolio repository
│   │   │   └── segment/      # Segment repository
│   │   ├── rabbitmq/     # RabbitMQ adapters
│   │   └── redis/        # Redis adapters
│   ├── bootstrap/        # Application bootstrapping
│   └── services/         # Business logic services
│       ├── command/      # Command services
│       └── query/        # Query services
└── migrations/           # Database migration scripts
```

### API

The Onboarding API is defined using OpenAPI specification:

```yaml
openapi: 3.0.1
info:
  title: Midaz Onboarding API
  version: v1.48.0
  description: This is a swagger documentation for the Midaz Ledger API
paths:
  /v1/organizations:
    get:
      description: Get all Organizations with the input metadata or without metadata
      # ...
    post:
      description: Create an Organization with the input payload
      # ...
  /v1/organizations/{id}:
    get:
      description: Get an Organization with the input ID
      # ...
    patch:
      description: Update an Organization with the input payload
      # ...
    delete:
      description: Delete an Organization with the input ID
      # ...
  # ... more endpoints for ledgers, accounts, assets, portfolios, segments
```

### Bootstrap

The Onboarding service is bootstrapped in the `main.go` file:

```go
func main() {
    libCommons.InitLocalEnvConfig()
    bootstrap.InitServers().Run()
}
```

The `bootstrap` package initializes the server and services:

```go
// Server represents the http server for Ledger service.
type Server struct {
    app           *fiber.App
    serverAddress string
    libLog.Logger
    libOpentelemetry.Telemetry
}

// Service is the application glue where we put all top level components to be used.
type Service struct {
    *Server
    libLog.Logger
}

// Run starts the application.
func (app *Service) Run() {
    libCommons.NewLauncher(
        libCommons.WithLogger(app.Logger),
        libCommons.RunApp("HTTP server", app.Server),
    ).Run()
}
```

## Transaction Component

The Transaction component handles all financial transactions in the Midaz system. It's located in the `components/transaction` directory.

### Purpose

- Process financial transactions
- Manage account balances
- Track asset rates
- Ensure financial integrity through double-entry accounting

### Key Files and Directories

- `api/`: API definitions and documentation
- `cmd/app/`: Application entry point
- `internal/adapters/`: Interface adapters
- `internal/bootstrap/`: Application bootstrapping
- `internal/services/`: Business logic services
- `migrations/`: Database migration scripts

### Architecture

Similar to the Onboarding service, the Transaction service follows a clean architecture pattern:

```
transaction/
├── api/                  # API definitions and documentation
├── cmd/                  # Application entry points
│   └── app/              # Main application
├── internal/             # Internal packages
│   ├── adapters/         # Interface adapters
│   │   ├── http/         # HTTP adapters
│   │   │   ├── in/       # Inbound HTTP adapters
│   │   │   └── out/      # Outbound HTTP adapters
│   │   ├── mongodb/      # MongoDB adapters
│   │   ├── postgres/     # PostgreSQL adapters
│   │   │   ├── assetrate/ # Asset rate repository
│   │   │   ├── balance/   # Balance repository
│   │   │   ├── operation/ # Operation repository
│   │   │   └── transaction/ # Transaction repository
│   │   ├── rabbitmq/     # RabbitMQ adapters
│   │   └── redis/        # Redis adapters
│   ├── bootstrap/        # Application bootstrapping
│   └── services/         # Business logic services
│       ├── command/      # Command services
│       └── query/        # Query services
└── migrations/           # Database migration scripts
```

### API

The Transaction API is defined using OpenAPI specification:

```yaml
openapi: 3.0.1
info:
  title: Midaz Transaction API
  version: v1.48.0
  description: This is a swagger documentation for the Midaz Transaction API
paths:
  /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances:
    get:
      description: Get all balances by account id
      # ...
  /v1/organizations/:organization_id/ledgers/:ledger_id/balances:
    get:
      description: Get all balances
      # ...
  /v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id:
    get:
      description: Get a Balance with the input ID
      # ...
    delete:
      description: Delete a Balance with the input ID
      # ...
  # ... more endpoints for operations, transactions, asset rates
```

### Transaction Model

The Transaction model represents a financial transaction:

```go
// Transaction is a struct designed to encapsulate response payload data.
type Transaction struct {
    ID                       string                     // Unique identifier
    ParentTransactionID      *string                    // Parent transaction ID (optional)
    Description              string                     // Transaction description
    Template                 string                     // Transaction template
    Status                   Status                     // Status
    Amount                   *int64                     // Transaction amount
    AmountScale              *int64                     // Amount scale (decimal places)
    AssetCode                string                     // Asset code (e.g., USD, BTC)
    ChartOfAccountsGroupName string                     // Chart of accounts group
    Source                   []string                   // Source accounts
    Destination              []string                   // Destination accounts
    LedgerID                 string                     // Parent ledger ID
    OrganizationID           string                     // Parent organization ID
    Body                     libTransaction.Transaction // Transaction body
    CreatedAt                time.Time                  // Creation timestamp
    UpdatedAt                time.Time                  // Last update timestamp
    DeletedAt                *time.Time                 // Deletion timestamp (optional)
    Metadata                 map[string]any             // Custom metadata
    Operations               []*operation.Operation     // Associated operations
}
```

### Double-Entry Accounting

The Transaction component ensures double-entry accounting principles are followed:

```go
// TransactionRevert is a func that revert transaction
func (t Transaction) TransactionRevert() libTransaction.Transaction {
    froms := make([]libTransaction.FromTo, 0)

    for _, to := range t.Body.Send.Distribute.To {
        to.IsFrom = true
        froms = append(froms, to)
    }

    newSource := libTransaction.Source{
        From: froms,
    }

    tos := make([]libTransaction.FromTo, 0)

    for _, from := range t.Body.Send.Source.From {
        from.IsFrom = false
        tos = append(tos, from)
    }

    newDistribute := libTransaction.Distribute{
        To: tos,
    }

    return libTransaction.Transaction{
        ChartOfAccountsGroupName: t.Body.ChartOfAccountsGroupName,
        Description:              t.Body.Description,
        Code:                     t.Body.Code,
        Pending:                  t.Body.Pending,
        Metadata:                 t.Body.Metadata,
        Send: libTransaction.Send{
            Source:     newSource,
            Distribute: newDistribute,
        },
    }
}
```

## Shared Packages

The `pkg` directory contains shared packages used by multiple components.

### Purpose

- Provide common functionality and models
- Ensure consistency across components
- Reduce code duplication

### Key Packages

#### mmodel

The `mmodel` package contains shared data models:

```go
// Organization is a struct designed to encapsulate response payload data.
type Organization struct {
    ID                   string         // Unique identifier
    ParentOrganizationID *string        // Parent organization ID (optional)
    LegalName            string         // Legal name
    DoingBusinessAs      *string        // DBA name (optional)
    LegalDocument        string         // Legal document (e.g., tax ID)
    Address              Address        // Physical address
    Status               Status         // Status (active, inactive, etc.)
    CreatedAt            time.Time      // Creation timestamp
    UpdatedAt            time.Time      // Last update timestamp
    DeletedAt            *time.Time     // Deletion timestamp (optional)
    Metadata             map[string]any // Custom metadata
}

// Ledger is a struct designed to encapsulate payload data.
type Ledger struct {
    ID             string         // Unique identifier
    Name           string         // Ledger name
    OrganizationID string         // Parent organization ID
    Status         Status         // Status (active, inactive, etc.)
    CreatedAt      time.Time      // Creation timestamp
    UpdatedAt      time.Time      // Last update timestamp
    DeletedAt      *time.Time     // Deletion timestamp (optional)
    Metadata       map[string]any // Custom metadata
}

// Account is a struct designed to encapsulate response payload data.
type Account struct {
    ID              string         // Unique identifier
    Name            string         // Account name
    ParentAccountID *string        // Parent account ID (optional)
    EntityID        *string        // Associated entity ID (optional)
    AssetCode       string         // Asset code (e.g., USD, BTC)
    OrganizationID  string         // Parent organization ID
    LedgerID        string         // Parent ledger ID
    PortfolioID     *string        // Associated portfolio ID (optional)
    SegmentID       *string        // Associated segment ID (optional)
    Status          Status         // Status (active, inactive, etc.)
    Alias           *string        // Account alias (optional)
    Type            string         // Account type
    CreatedAt       time.Time      // Creation timestamp
    UpdatedAt       time.Time      // Last update timestamp
    DeletedAt       *time.Time     // Deletion timestamp (optional)
    Metadata        map[string]any // Custom metadata
}
```

#### constant

The `constant` package contains shared constants and error definitions:

```go
// Error codes
const (
    ErrCodeInvalidRequest           = "INVALID_REQUEST"
    ErrCodeInternalServerError      = "INTERNAL_SERVER_ERROR"
    ErrCodeNotFound                 = "NOT_FOUND"
    ErrCodeUnauthorized             = "UNAUTHORIZED"
    ErrCodeForbidden                = "FORBIDDEN"
    ErrCodeConflict                 = "CONFLICT"
    ErrCodeUnprocessableEntity      = "UNPROCESSABLE_ENTITY"
    ErrCodeTooManyRequests          = "TOO_MANY_REQUESTS"
    ErrCodeBadGateway               = "BAD_GATEWAY"
    ErrCodeServiceUnavailable       = "SERVICE_UNAVAILABLE"
    ErrCodeGatewayTimeout           = "GATEWAY_TIMEOUT"
    // ...
)
```

#### gold

The `gold` package contains utilities for transaction processing:

```go
// Transaction represents a financial transaction
type Transaction struct {
    ChartOfAccountsGroupName string               // Chart of accounts group name
    Description              string               // Transaction description
    Code                     string               // Transaction code
    Pending                  bool                 // Whether the transaction is pending
    Metadata                 map[string]any       // Custom metadata
    Send                     Send                 // Send details
}

// Send represents the sending details of a transaction
type Send struct {
    Source     Source     // Source accounts
    Distribute Distribute // Destination accounts
}

// Source represents the source accounts of a transaction
type Source struct {
    From []FromTo // Source accounts
}

// Distribute represents the destination accounts of a transaction
type Distribute struct {
    To []FromTo // Destination accounts
}

// FromTo represents an account involved in a transaction
type FromTo struct {
    AccountID   string // Account ID
    AccountType string // Account type
    Amount      int64  // Amount
    Scale       int64  // Scale (decimal places)
    AssetCode   string // Asset code
    IsFrom      bool   // Whether this is a source account
}
```
