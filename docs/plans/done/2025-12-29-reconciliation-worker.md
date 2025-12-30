# Reconciliation Worker Implementation Plan

**Goal**: Create a continuous reconciliation worker that monitors database consistency across PostgreSQL and MongoDB replicas

**Architecture**: Standalone background service with HTTP status endpoints
**Tech Stack**: Go 1.24, PostgreSQL (replica), MongoDB, OpenTelemetry
**Prerequisites**: Existing Midaz infrastructure running (postgres-primary, postgres-replica, mongodb)

---

## 🔴 ISOLATION REQUIREMENTS

**This component MUST be fully isolated for easy cherry-pick to main:**

| Dependency Type | Allowed? | Notes |
|----------------|----------|-------|
| `lib-commons` | ✅ YES | External stable library |
| `pkg/*` | ❌ NO | Inline any needed helpers |
| `components/transaction/*` | ❌ NO | Direct SQL queries only |
| `components/onboarding/*` | ❌ NO | Direct SQL queries only |
| Standard library | ✅ YES | |
| External Go libs | ✅ YES | fiber, pq, mongo-driver |

**Result**: `components/reconciliation/` can be copied as-is to any branch.

---

## File Structure (Self-Contained)

```
components/reconciliation/
├── cmd/
│   └── app/
│       └── main.go                    # Entry point (4 lines)
│
├── internal/
│   ├── adapters/
│   │   ├── postgres/
│   │   │   ├── models.go              # Data structures
│   │   │   ├── report.go              # Report aggregate
│   │   │   ├── connection.go          # DB connection wrapper
│   │   │   ├── settlement.go          # Settlement detection
│   │   │   ├── balance_check.go       # Balance consistency
│   │   │   ├── double_entry_check.go  # Double-entry validation
│   │   │   ├── orphan_check.go        # Orphan transactions
│   │   │   ├── referential_check.go   # Referential integrity
│   │   │   └── sync_check.go          # Redis-PG sync
│   │   │
│   │   ├── mongodb/
│   │   │   ├── connection.go          # MongoDB connection
│   │   │   └── metadata_check.go      # Metadata sync check
│   │   │
│   │   └── metrics/
│   │       └── metrics.go             # OpenTelemetry metrics
│   │
│   ├── domain/
│   │   └── report.go                  # Domain types (no deps)
│   │
│   ├── engine/
│   │   └── reconciliation.go          # Orchestrates all checks
│   │
│   └── bootstrap/
│       ├── config.go                  # Environment config
│       ├── service.go                 # Service + Run()
│       ├── worker.go                  # Background ticker
│       └── server.go                  # HTTP status API
│
├── pkg/                               # INTERNAL utilities (inlined)
│   └── safego/
│       └── safego.go                  # Panic-safe goroutines
│
├── .env.example
├── .air.toml
├── Dockerfile
├── Dockerfile.dev
├── docker-compose.yml
├── docker-compose.dev.yml
├── Makefile
├── go.mod                             # OPTIONAL: separate module
└── README.md                          # Quick start guide
```

---

## Task Breakdown

### Phase 1: Component Scaffolding (Tasks 1-6)

#### Task 1: Create directory structure
**File**: Multiple directories
**Time**: 2 min
**Agent**: `general-purpose`

```bash
# Run from repository root
mkdir -p components/reconciliation/{cmd/app,internal/{adapters/{postgres,mongodb},domain,engine,bootstrap},pkg/safego}
```

**Verification**:
```bash
tree components/reconciliation -L 4
```

---

#### Task 2: Create inlined SafeGo utility (NO pkg/ dependency)
**File**: `components/reconciliation/pkg/safego/safego.go`
**Time**: 3 min
**Agent**: `backend-engineer-golang`

```go
package safego

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	panicCounter metric.Int64Counter
	meterOnce    sync.Once
)

func initMeter() {
	meterOnce.Do(func() {
		meter := otel.Meter("reconciliation")
		panicCounter, _ = meter.Int64Counter(
			"reconciliation.panics.recovered",
			metric.WithDescription("Number of recovered panics"),
		)
	})
}

// Logger interface for panic logging
type Logger interface {
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
}

// sanitizePanicValue removes potential secrets from panic messages
func sanitizePanicValue(r interface{}) string {
	s := fmt.Sprintf("%v", r)
	// Redact patterns that look like credentials
	s = regexp.MustCompile(`password=\S+`).ReplaceAllString(s, "password=REDACTED")
	s = regexp.MustCompile(`://[^@]+@`).ReplaceAllString(s, "://REDACTED@")
	return s
}

// Go runs a function in a goroutine with panic recovery
func Go(logger Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				initMeter()
				if panicCounter != nil {
					panicCounter.Add(context.Background(), 1,
						metric.WithAttributes(attribute.String("goroutine", name)))
				}
				if logger != nil {
					// Log sanitized panic value (no stack trace to avoid info disclosure)
					logger.Errorf("panic recovered in goroutine %s: %v", name, sanitizePanicValue(r))
				}
			}
		}()
		fn()
	}()
}

// GoWithContext runs a function with context and panic recovery
func GoWithContext(ctx context.Context, logger Logger, name string, fn func(context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				initMeter()
				if panicCounter != nil {
					panicCounter.Add(ctx, 1,
						metric.WithAttributes(attribute.String("goroutine", name)))
				}
				if logger != nil {
					// Log sanitized panic value (no stack trace to avoid info disclosure)
					logger.Errorf("panic recovered in goroutine %s: %v", name, sanitizePanicValue(r))
				}
			}
		}()
		fn(ctx)
	}()
}
```

**Verification**:
```bash
go build ./components/reconciliation/pkg/safego/
```

---

#### Task 3: Create main.go entry point
**File**: `components/reconciliation/cmd/app/main.go`
**Time**: 2 min
**Agent**: `backend-engineer-golang`

```go
package main

import (
	"github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/bootstrap"
)

func main() {
	commons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
```

**Verification**:
```bash
head -15 components/reconciliation/cmd/app/main.go
```

---

#### Task 4: Create config.go with environment variables
**File**: `components/reconciliation/internal/bootstrap/config.go`
**Time**: 5 min
**Agent**: `backend-engineer-golang`

```go
package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

const ApplicationName = "reconciliation"

// Config holds all configuration for the reconciliation worker
type Config struct {
	EnvName   string `env:"ENV_NAME"`
	LogLevel  string `env:"LOG_LEVEL"`
	Version   string `env:"VERSION"`

	// HTTP Server (for status endpoints)
	ServerAddress string `env:"SERVER_ADDRESS"`

	// PostgreSQL Replica (READ-ONLY) - Onboarding DB
	OnboardingDBHost     string `env:"ONBOARDING_DB_HOST"`
	OnboardingDBUser     string `env:"ONBOARDING_DB_USER"`
	OnboardingDBPassword string `env:"ONBOARDING_DB_PASSWORD"`
	OnboardingDBName     string `env:"ONBOARDING_DB_NAME"`
	OnboardingDBPort     string `env:"ONBOARDING_DB_PORT"`
	OnboardingDBSSLMode  string `env:"ONBOARDING_DB_SSLMODE"`

	// PostgreSQL Replica (READ-ONLY) - Transaction DB
	TransactionDBHost     string `env:"TRANSACTION_DB_HOST"`
	TransactionDBUser     string `env:"TRANSACTION_DB_USER"`
	TransactionDBPassword string `env:"TRANSACTION_DB_PASSWORD"`
	TransactionDBName     string `env:"TRANSACTION_DB_NAME"`
	TransactionDBPort     string `env:"TRANSACTION_DB_PORT"`
	TransactionDBSSLMode  string `env:"TRANSACTION_DB_SSLMODE"`

	// MongoDB
	MongoHost     string `env:"MONGO_HOST"`
	MongoUser     string `env:"MONGO_USER"`
	MongoPassword string `env:"MONGO_PASSWORD"`
	MongoPort     string `env:"MONGO_PORT"`
	OnboardingMongoDBName  string `env:"ONBOARDING_MONGO_DB_NAME"`
	TransactionMongoDBName string `env:"TRANSACTION_MONGO_DB_NAME"`

	// Worker Configuration
	ReconciliationIntervalSeconds int   `env:"RECONCILIATION_INTERVAL_SECONDS"`
	SettlementWaitSeconds         int   `env:"SETTLEMENT_WAIT_SECONDS"`
	DiscrepancyThreshold          int64 `env:"DISCREPANCY_THRESHOLD"`
	MaxDiscrepanciesToReport      int   `env:"MAX_DISCREPANCIES_TO_REPORT"`

	// Telemetry
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`

	// Connection Pool
	MaxOpenConnections int `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections int `env:"DB_MAX_IDLE_CONNS"`
}

// GetReconciliationInterval returns the interval as duration
func (c *Config) GetReconciliationInterval() time.Duration {
	if c.ReconciliationIntervalSeconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(c.ReconciliationIntervalSeconds) * time.Second
}

// Validate checks configuration values for correctness
func (c *Config) Validate() error {
	if c.ReconciliationIntervalSeconds < 60 {
		return fmt.Errorf("RECONCILIATION_INTERVAL_SECONDS must be >= 60 (got %d)", c.ReconciliationIntervalSeconds)
	}
	if c.MaxDiscrepanciesToReport < 1 || c.MaxDiscrepanciesToReport > 1000 {
		return fmt.Errorf("MAX_DISCREPANCIES_TO_REPORT must be between 1 and 1000 (got %d)", c.MaxDiscrepanciesToReport)
	}
	if c.MaxOpenConnections < 1 || c.MaxOpenConnections > 100 {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be between 1 and 100 (got %d)", c.MaxOpenConnections)
	}
	if c.OnboardingDBSSLMode == "disable" && c.EnvName == "production" {
		return fmt.Errorf("ONBOARDING_DB_SSLMODE=disable is not allowed in production")
	}
	if c.TransactionDBSSLMode == "disable" && c.EnvName == "production" {
		return fmt.Errorf("TRANSACTION_DB_SSLMODE=disable is not allowed in production")
	}
	return nil
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ReconciliationIntervalSeconds: 300, // 5 minutes
		SettlementWaitSeconds:         300, // 5 minutes
		DiscrepancyThreshold:          0,   // Report any discrepancy
		MaxDiscrepanciesToReport:      100,
		MaxOpenConnections:            10,
		MaxIdleConnections:            5,
		OnboardingDBSSLMode:           "require", // Secure default
		TransactionDBSSLMode:          "require", // Secure default
	}
}

// sanitizeConnectionError removes credentials from error messages
func sanitizeConnectionError(msg string) string {
	msg = regexp.MustCompile(`password=\S+`).ReplaceAllString(msg, "password=REDACTED")
	msg = regexp.MustCompile(`://[^@]+@`).ReplaceAllString(msg, "://REDACTED@")
	return msg
}

// InitServers initializes all components and returns the service
func InitServers() *Service {
	service, err := InitServersWithOptions(nil)
	if err != nil {
		// Sanitize error to prevent credential leakage in logs/panic output
		panic(fmt.Sprintf("reconciliation.InitServers failed: %v", sanitizeConnectionError(err.Error())))
	}
	return service
}

// Options allows injecting dependencies
type Options struct {
	Logger libLog.Logger
}

// InitServersWithOptions initializes with optional dependencies
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := DefaultConfig()
	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Logger
	var logger libLog.Logger
	if opts != nil && opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = libLog.NewLogrus(cfg.LogLevel, cfg.EnvName == "production")
	}

	// Telemetry
	telemetry := libOpentelemetry.InitializeTelemetry(
		cfg.OtelServiceName,
		cfg.OtelLibraryName,
		cfg.OtelServiceVersion,
		cfg.OtelDeploymentEnv,
		cfg.OtelColExporterEndpoint,
		cfg.EnableTelemetry,
		logger,
	)

	// PostgreSQL connections (direct, no lib-commons wrapper for isolation)
	onboardingDB, err := connectPostgres(
		cfg.OnboardingDBHost, cfg.OnboardingDBPort, cfg.OnboardingDBUser,
		cfg.OnboardingDBPassword, cfg.OnboardingDBName, cfg.OnboardingDBSSLMode,
		cfg.MaxOpenConnections, cfg.MaxIdleConnections,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to onboarding DB: %w", err)
	}

	transactionDB, err := connectPostgres(
		cfg.TransactionDBHost, cfg.TransactionDBPort, cfg.TransactionDBUser,
		cfg.TransactionDBPassword, cfg.TransactionDBName, cfg.TransactionDBSSLMode,
		cfg.MaxOpenConnections, cfg.MaxIdleConnections,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to transaction DB: %w", err)
	}

	// MongoDB connections
	mongoClient, err := connectMongo(cfg.MongoHost, cfg.MongoPort, cfg.MongoUser, cfg.MongoPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	onboardingMongoDB := mongoClient.Database(cfg.OnboardingMongoDBName)
	transactionMongoDB := mongoClient.Database(cfg.TransactionMongoDBName)

	// Initialize engine with individual config values
	engine := NewReconciliationEngine(
		onboardingDB,
		transactionDB,
		onboardingMongoDB,
		transactionMongoDB,
		logger,
		cfg.DiscrepancyThreshold,
		cfg.MaxDiscrepanciesToReport,
		cfg.SettlementWaitSeconds,
	)

	// Initialize worker
	worker := NewReconciliationWorker(engine, logger, cfg)

	// Initialize HTTP server
	httpServer := NewHTTPServer(cfg.ServerAddress, engine, logger, telemetry, cfg.Version, cfg.EnvName)

	return &Service{
		Worker:     worker,
		HTTPServer: httpServer,
		Logger:     logger,
		Config:     cfg,
	}, nil
}

// connectPostgres creates a direct PostgreSQL connection
func connectPostgres(host, port, user, password, dbname, sslmode string, maxOpen, maxIdle int) (*sql.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s statement_timeout=30000 lock_timeout=10000",
		host, port, user, password, dbname, sslmode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	return db, nil
}

// connectMongo creates a direct MongoDB connection with TLS
func connectMongo(host, port, user, password string) (*mongo.Client, error) {
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s/?authSource=admin&directConnection=true&tls=true",
		user, password, host, port)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	return client, nil
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/bootstrap/
```

---

#### Task 5: Create service.go with launcher
**File**: `components/reconciliation/internal/bootstrap/service.go`
**Time**: 3 min
**Agent**: `backend-engineer-golang`

```go
package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Service holds all components for the reconciliation worker
type Service struct {
	Worker     *ReconciliationWorker
	HTTPServer *HTTPServer
	Logger     libLog.Logger
	Config     *Config
}

// Run starts all workers using the launcher pattern
func (s *Service) Run() {
	opts := []libCommons.LauncherOption{
		libCommons.WithLogger(s.Logger),
		libCommons.RunApp("Reconciliation Worker", s.Worker),
		libCommons.RunApp("HTTP Status Server", s.HTTPServer),
	}
	libCommons.NewLauncher(opts...).Run()
}
```

**Verification**:
```bash
grep -n "func.*Run" components/reconciliation/internal/bootstrap/service.go
```

---

#### Task 6: Create .env.example
**File**: `components/reconciliation/.env.example`
**Time**: 2 min
**Agent**: `backend-engineer-golang`

```bash
# Reconciliation Worker Configuration
ENV_NAME=development
VERSION=v1.0.0
LOG_LEVEL=info
SERVER_ADDRESS=:3005

# PostgreSQL REPLICA - Onboarding DB (READ-ONLY)
ONBOARDING_DB_HOST=midaz-postgres-replica
ONBOARDING_DB_USER=midaz
ONBOARDING_DB_PASSWORD=lerian
ONBOARDING_DB_NAME=onboarding
ONBOARDING_DB_PORT=5702
ONBOARDING_DB_SSLMODE=require

# PostgreSQL REPLICA - Transaction DB (READ-ONLY)
TRANSACTION_DB_HOST=midaz-postgres-replica
TRANSACTION_DB_USER=midaz
TRANSACTION_DB_PASSWORD=lerian
TRANSACTION_DB_NAME=transaction
TRANSACTION_DB_PORT=5702
TRANSACTION_DB_SSLMODE=require

# MongoDB (Metadata)
MONGO_HOST=midaz-mongodb
MONGO_USER=midaz
MONGO_PASSWORD=lerian
MONGO_PORT=5703
ONBOARDING_MONGO_DB_NAME=onboarding
TRANSACTION_MONGO_DB_NAME=transaction

# Worker Configuration
RECONCILIATION_INTERVAL_SECONDS=300
SETTLEMENT_WAIT_SECONDS=300
DISCREPANCY_THRESHOLD=0
MAX_DISCREPANCIES_TO_REPORT=100

# Connection Pool
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=5

# Telemetry
OTEL_RESOURCE_SERVICE_NAME=reconciliation
OTEL_LIBRARY_NAME=github.com/LerianStudio/midaz/components/reconciliation
OTEL_RESOURCE_SERVICE_VERSION=${VERSION}
OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT=${ENV_NAME}
OTEL_EXPORTER_OTLP_ENDPOINT=midaz-otel-collector:4317
ENABLE_TELEMETRY=true
```

**Verification**:
```bash
wc -l components/reconciliation/.env.example
```

---

### Phase 1 Review Checkpoint
**Severity**: Low (scaffolding only)
**Action**: Verify no imports from `pkg/` or other components

---

### Phase 2: Domain Models (Tasks 7-9)

#### Task 7: Create domain report types (zero dependencies)
**File**: `components/reconciliation/internal/domain/report.go`
**Time**: 5 min
**Agent**: `backend-engineer-golang`

```go
package domain

import "time"

// ReconciliationReport is the complete output of a reconciliation run
type ReconciliationReport struct {
	Timestamp   time.Time `json:"timestamp"`
	Duration    string    `json:"duration"`
	Status      string    `json:"status"` // HEALTHY, WARNING, CRITICAL

	// Settlement info
	UnsettledTransactions int `json:"unsettled_transactions"`
	SettledTransactions   int `json:"settled_transactions"`

	// Check results
	BalanceCheck      *BalanceCheckResult      `json:"balance_check"`
	DoubleEntryCheck  *DoubleEntryCheckResult  `json:"double_entry_check"`
	ReferentialCheck  *ReferentialCheckResult  `json:"referential_check"`
	SyncCheck         *SyncCheckResult         `json:"sync_check"`
	OrphanCheck       *OrphanCheckResult       `json:"orphan_check"`
	MetadataCheck     *MetadataCheckResult     `json:"metadata_check"`

	// Entity counts
	EntityCounts *EntityCounts `json:"entity_counts"`
}

// BalanceCheckResult holds balance consistency results
type BalanceCheckResult struct {
	TotalBalances            int                  `json:"total_balances"`
	BalancesWithDiscrepancy  int                  `json:"balances_with_discrepancy"`
	DiscrepancyPercentage    float64              `json:"discrepancy_percentage"`
	TotalAbsoluteDiscrepancy int64                `json:"total_absolute_discrepancy"`
	Discrepancies            []BalanceDiscrepancy `json:"discrepancies,omitempty"`
	Status                   string               `json:"status"`
}

// BalanceDiscrepancy represents a balance that doesn't match its operations
type BalanceDiscrepancy struct {
	BalanceID       string    `json:"balance_id"`
	AccountID       string    `json:"account_id"`
	Alias           string    `json:"alias"`
	AssetCode       string    `json:"asset_code"`
	CurrentBalance  int64     `json:"current_balance"`
	ExpectedBalance int64     `json:"expected_balance"`
	Discrepancy     int64     `json:"discrepancy"`
	OperationCount  int64     `json:"operation_count"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DoubleEntryCheckResult holds double-entry validation results
type DoubleEntryCheckResult struct {
	TotalTransactions        int                    `json:"total_transactions"`
	UnbalancedTransactions   int                    `json:"unbalanced_transactions"`
	UnbalancedPercentage     float64                `json:"unbalanced_percentage"`
	TransactionsNoOperations int                    `json:"transactions_without_operations"`
	Imbalances               []TransactionImbalance `json:"imbalances,omitempty"`
	Status                   string                 `json:"status"`
}

// TransactionImbalance represents a transaction where credits != debits
type TransactionImbalance struct {
	TransactionID  string   `json:"transaction_id"`
	Status         string   `json:"status"`
	AssetCode      string   `json:"asset_code"`
	TotalCredits   int64    `json:"total_credits"`
	TotalDebits    int64    `json:"total_debits"`
	Imbalance      int64    `json:"imbalance"`
	OperationCount int64    `json:"operation_count"`
}

// ReferentialCheckResult holds referential integrity results
type ReferentialCheckResult struct {
	OrphanLedgers    int            `json:"orphan_ledgers"`
	OrphanAssets     int            `json:"orphan_assets"`
	OrphanAccounts   int            `json:"orphan_accounts"`
	OrphanOperations int            `json:"orphan_operations"`
	OrphanPortfolios int            `json:"orphan_portfolios"`
	Orphans          []OrphanEntity `json:"orphans,omitempty"`
	Status           string         `json:"status"`
}

// OrphanEntity represents an entity with missing references
type OrphanEntity struct {
	EntityType    string `json:"entity_type"`
	EntityID      string `json:"entity_id"`
	ReferenceType string `json:"reference_type"`
	ReferenceID   string `json:"reference_id"`
}

// SyncCheckResult holds Redis-PostgreSQL sync results
type SyncCheckResult struct {
	VersionMismatches int         `json:"version_mismatches"`
	StaleBalances     int         `json:"stale_balances"`
	Issues            []SyncIssue `json:"issues,omitempty"`
	Status            string      `json:"status"`
}

// SyncIssue represents a Redis-PostgreSQL sync problem
type SyncIssue struct {
	BalanceID        string `json:"balance_id"`
	Alias            string `json:"alias"`
	AssetCode        string `json:"asset_code"`
	DBVersion        int32  `json:"db_version"`
	MaxOpVersion     int32  `json:"max_op_version"`
	StalenessSeconds int64  `json:"staleness_seconds"`
}

// OrphanCheckResult holds orphan transaction results
type OrphanCheckResult struct {
	OrphanTransactions  int                 `json:"orphan_transactions"`
	PartialTransactions int                 `json:"partial_transactions"`
	Orphans             []OrphanTransaction `json:"orphans,omitempty"`
	Status              string              `json:"status"`
}

// OrphanTransaction represents a transaction without operations
type OrphanTransaction struct {
	TransactionID  string    `json:"transaction_id"`
	OrganizationID string    `json:"organization_id"`
	LedgerID       string    `json:"ledger_id"`
	Status         string    `json:"status"`
	Amount         int64     `json:"amount"`
	AssetCode      string    `json:"asset_code"`
	CreatedAt      time.Time `json:"created_at"`
	OperationCount int32     `json:"operation_count"`
}

// MetadataCheckResult holds metadata sync results
type MetadataCheckResult struct {
	PostgreSQLCount int64  `json:"postgresql_count"`
	MongoDBCount    int64  `json:"mongodb_count"`
	MissingCount    int64  `json:"missing_count"`
	Status          string `json:"status"`
}

// EntityCounts holds PostgreSQL entity counts
type EntityCounts struct {
	// Onboarding
	Organizations int64 `json:"organizations"`
	Ledgers       int64 `json:"ledgers"`
	Assets        int64 `json:"assets"`
	Accounts      int64 `json:"accounts"`
	Portfolios    int64 `json:"portfolios"`

	// Transaction
	Transactions int64 `json:"transactions"`
	Operations   int64 `json:"operations"`
	Balances     int64 `json:"balances"`
}

// DetermineOverallStatus calculates the overall status from individual checks
func (r *ReconciliationReport) DetermineOverallStatus() {
	criticalCount := 0
	warningCount := 0

	checkStatuses := []string{}
	if r.BalanceCheck != nil {
		checkStatuses = append(checkStatuses, r.BalanceCheck.Status)
	}
	if r.DoubleEntryCheck != nil {
		checkStatuses = append(checkStatuses, r.DoubleEntryCheck.Status)
	}
	if r.ReferentialCheck != nil {
		checkStatuses = append(checkStatuses, r.ReferentialCheck.Status)
	}
	if r.SyncCheck != nil {
		checkStatuses = append(checkStatuses, r.SyncCheck.Status)
	}
	if r.OrphanCheck != nil {
		checkStatuses = append(checkStatuses, r.OrphanCheck.Status)
	}
	if r.MetadataCheck != nil {
		checkStatuses = append(checkStatuses, r.MetadataCheck.Status)
	}

	for _, status := range checkStatuses {
		switch status {
		case "CRITICAL":
			criticalCount++
		case "WARNING":
			warningCount++
		}
	}

	if criticalCount > 0 {
		r.Status = "CRITICAL"
	} else if warningCount > 0 {
		r.Status = "WARNING"
	} else {
		r.Status = "HEALTHY"
	}
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/domain/
# Should have ZERO imports from midaz packages
grep -r "LerianStudio/midaz" components/reconciliation/internal/domain/
```

---

#### Task 8: Create PostgreSQL adapter models
**File**: `components/reconciliation/internal/adapters/postgres/models.go`
**Time**: 3 min
**Agent**: `backend-engineer-golang`

```go
package postgres

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Scanner interface for database rows
type Scanner interface {
	Scan(dest ...interface{}) error
}

// BalanceRow represents a row from balance consistency query
type BalanceRow struct {
	BalanceID       string
	AccountID       string
	Alias           string
	AssetCode       string
	CurrentBalance  int64
	TotalCredits    int64
	TotalDebits     int64
	ExpectedBalance int64
	Discrepancy     int64
	OperationCount  int64
	UpdatedAt       time.Time
}

// ToDomain converts to domain type
func (r *BalanceRow) ToDomain() domain.BalanceDiscrepancy {
	return domain.BalanceDiscrepancy{
		BalanceID:       r.BalanceID,
		AccountID:       r.AccountID,
		Alias:           r.Alias,
		AssetCode:       r.AssetCode,
		CurrentBalance:  r.CurrentBalance,
		ExpectedBalance: r.ExpectedBalance,
		Discrepancy:     r.Discrepancy,
		OperationCount:  r.OperationCount,
		UpdatedAt:       r.UpdatedAt,
	}
}

// TransactionRow represents a row from double-entry query
type TransactionRow struct {
	TransactionID  string
	Status         string
	AssetCode      string
	TotalCredits   int64
	TotalDebits    int64
	Imbalance      int64
	OperationCount int64
}

// ToDomain converts to domain type
func (r *TransactionRow) ToDomain() domain.TransactionImbalance {
	return domain.TransactionImbalance{
		TransactionID:  r.TransactionID,
		Status:         r.Status,
		AssetCode:      r.AssetCode,
		TotalCredits:   r.TotalCredits,
		TotalDebits:    r.TotalDebits,
		Imbalance:      r.Imbalance,
		OperationCount: r.OperationCount,
	}
}

// NullableString handles nullable database strings
func NullableString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// NullableInt64 handles nullable database int64
func NullableInt64(ni sql.NullInt64) int64 {
	if ni.Valid {
		return ni.Int64
	}
	return 0
}

// NullableTime handles nullable database time
func NullableTime(nt sql.NullTime) time.Time {
	if nt.Valid {
		return nt.Time
	}
	return time.Time{}
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/adapters/postgres/
```

---

### Phase 2 Review Checkpoint
**Severity**: Medium
**Action**: Verify domain package has zero external deps

---

### Phase 3: Check Implementations (Tasks 9-14)

#### Task 9: Balance consistency check
**File**: `components/reconciliation/internal/adapters/postgres/balance_check.go`
**Time**: 5 min
**Agent**: `backend-engineer-golang`

```go
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// BalanceChecker performs balance consistency checks
type BalanceChecker struct {
	db *sql.DB
}

// NewBalanceChecker creates a new balance checker
func NewBalanceChecker(db *sql.DB) *BalanceChecker {
	return &BalanceChecker{db: db}
}

// Check verifies balance = sum(credits) - sum(debits)
func (c *BalanceChecker) Check(ctx context.Context, threshold int64, limit int) (*domain.BalanceCheckResult, error) {
	result := &domain.BalanceCheckResult{}

	// Summary query - using explicit DECIMAL cast for comparison
	summaryQuery := `
		WITH balance_calc AS (
			SELECT
				b.id,
				b.available::DECIMAL as current_balance,
				COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_credits,
				COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_debits,
				COUNT(o.id) as operation_count
			FROM balance b
			LEFT JOIN operation o ON b.account_id = o.account_id
				AND b.asset_code = o.asset_code
				AND b.key = o.balance_key
				AND o.deleted_at IS NULL
			WHERE b.deleted_at IS NULL
			GROUP BY b.id, b.available
		)
		SELECT
			COUNT(*) as total_balances,
			COUNT(*) FILTER (WHERE ABS(current_balance - (total_credits - total_debits)) > $1) as discrepancies,
			COALESCE(SUM(ABS(current_balance - (total_credits - total_debits)))
				FILTER (WHERE ABS(current_balance - (total_credits - total_debits)) > $1), 0)::BIGINT as total_discrepancy
		FROM balance_calc
	`

	var totalDiscrepancy int64
	err := c.db.QueryRowContext(ctx, summaryQuery, threshold).Scan(
		&result.TotalBalances,
		&result.BalancesWithDiscrepancy,
		&totalDiscrepancy,
	)
	if err != nil {
		return nil, fmt.Errorf("balance summary query failed: %w", err)
	}

	result.TotalAbsoluteDiscrepancy = totalDiscrepancy
	if result.TotalBalances > 0 {
		result.DiscrepancyPercentage = float64(result.BalancesWithDiscrepancy) / float64(result.TotalBalances) * 100
	}

	// Determine status
	if result.BalancesWithDiscrepancy == 0 {
		result.Status = "HEALTHY"
	} else if result.BalancesWithDiscrepancy <= 10 {
		result.Status = "WARNING"
	} else {
		result.Status = "CRITICAL"
	}

	// Get detailed discrepancies
	if result.BalancesWithDiscrepancy > 0 && limit > 0 {
		detailQuery := `
			WITH balance_calc AS (
				SELECT
					b.id as balance_id,
					b.account_id,
					a.alias,
					b.asset_code,
					b.available::DECIMAL as current_balance,
					COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_credits,
					COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_debits,
					COUNT(o.id) as operation_count,
					b.updated_at
				FROM balance b
				JOIN account a ON b.account_id = a.id
				LEFT JOIN operation o ON b.account_id = o.account_id
					AND b.asset_code = o.asset_code
					AND b.key = o.balance_key
					AND o.deleted_at IS NULL
				WHERE b.deleted_at IS NULL
				GROUP BY b.id, b.account_id, a.alias, b.asset_code, b.available, b.updated_at
			)
			SELECT
				balance_id, account_id, alias, asset_code,
				current_balance::BIGINT, (total_credits - total_debits)::BIGINT as expected_balance,
				(current_balance - (total_credits - total_debits))::BIGINT as discrepancy,
				operation_count, updated_at
			FROM balance_calc
			WHERE ABS(current_balance - (total_credits - total_debits)) > $1
			ORDER BY ABS(current_balance - (total_credits - total_debits)) DESC
			LIMIT $2
		`

		rows, err := c.db.QueryContext(ctx, detailQuery, threshold, limit)
		if err != nil {
			return nil, fmt.Errorf("balance detail query failed: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var d domain.BalanceDiscrepancy
			err := rows.Scan(
				&d.BalanceID, &d.AccountID, &d.Alias, &d.AssetCode,
				&d.CurrentBalance, &d.ExpectedBalance, &d.Discrepancy,
				&d.OperationCount, &d.UpdatedAt,
			)
			if err != nil {
				return nil, fmt.Errorf("balance row scan failed: %w", err)
			}
			result.Discrepancies = append(result.Discrepancies, d)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("balance row iteration failed: %w", err)
		}
	}

	return result, nil
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/adapters/postgres/
```

---

#### Task 10: Double-entry validation check
**File**: `components/reconciliation/internal/adapters/postgres/double_entry_check.go`
**Time**: 4 min
**Agent**: `backend-engineer-golang`

```go
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// DoubleEntryChecker validates credits = debits for transactions
type DoubleEntryChecker struct {
	db *sql.DB
}

// NewDoubleEntryChecker creates a new double-entry checker
func NewDoubleEntryChecker(db *sql.DB) *DoubleEntryChecker {
	return &DoubleEntryChecker{db: db}
}

// Check verifies every transaction has balanced credits and debits
// NOTE: ALL transactions (including NOTED) must have balanced operations for audit integrity
func (c *DoubleEntryChecker) Check(ctx context.Context, limit int) (*domain.DoubleEntryCheckResult, error) {
	result := &domain.DoubleEntryCheckResult{}

	// Summary query - only exclude PENDING (no operations yet)
	// NOTED transactions must still balance for audit integrity
	summaryQuery := `
		WITH transaction_balance AS (
			SELECT
				t.id,
				t.status,
				COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) as total_credits,
				COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) as total_debits,
				COUNT(o.id) as operation_count
			FROM transaction t
			LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
			WHERE t.deleted_at IS NULL
			  AND t.status != 'PENDING'
			GROUP BY t.id, t.status
		)
		SELECT
			COUNT(*) as total_transactions,
			COUNT(*) FILTER (WHERE total_credits != total_debits) as unbalanced,
			COUNT(*) FILTER (WHERE operation_count = 0) as no_operations
		FROM transaction_balance
	`

	err := c.db.QueryRowContext(ctx, summaryQuery).Scan(
		&result.TotalTransactions,
		&result.UnbalancedTransactions,
		&result.TransactionsNoOperations,
	)
	if err != nil {
		return nil, fmt.Errorf("double-entry summary query failed: %w", err)
	}

	if result.TotalTransactions > 0 {
		result.UnbalancedPercentage = float64(result.UnbalancedTransactions) / float64(result.TotalTransactions) * 100
	}

	// Status - unbalanced transactions are CRITICAL
	if result.UnbalancedTransactions == 0 {
		result.Status = "HEALTHY"
	} else {
		result.Status = "CRITICAL"
	}

	// Get detailed imbalances
	if result.UnbalancedTransactions > 0 && limit > 0 {
		detailQuery := `
			WITH transaction_balance AS (
				SELECT
					t.id as transaction_id,
					t.status,
					t.asset_code,
					COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) as total_credits,
					COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) as total_debits,
					COUNT(o.id) as operation_count
				FROM transaction t
				LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
				WHERE t.deleted_at IS NULL
				  AND t.status != 'PENDING'
				GROUP BY t.id, t.status, t.asset_code
			)
			SELECT
				transaction_id, status, asset_code,
				total_credits, total_debits,
				(total_credits - total_debits) as imbalance,
				operation_count
			FROM transaction_balance
			WHERE total_credits != total_debits
			ORDER BY ABS(total_credits - total_debits) DESC
			LIMIT $1
		`

		rows, err := c.db.QueryContext(ctx, detailQuery, limit)
		if err != nil {
			return nil, fmt.Errorf("double-entry detail query failed: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var i domain.TransactionImbalance
			err := rows.Scan(
				&i.TransactionID, &i.Status, &i.AssetCode,
				&i.TotalCredits, &i.TotalDebits, &i.Imbalance, &i.OperationCount,
			)
			if err != nil {
				return nil, fmt.Errorf("double-entry row scan failed: %w", err)
			}
			result.Imbalances = append(result.Imbalances, i)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("double-entry row iteration failed: %w", err)
		}
	}

	return result, nil
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/adapters/postgres/
```

---

#### Task 11: Orphan transaction check
**File**: `components/reconciliation/internal/adapters/postgres/orphan_check.go`
**Time**: 4 min
**Agent**: `backend-engineer-golang`

```go
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// OrphanChecker finds transactions without operations
type OrphanChecker struct {
	db *sql.DB
}

// NewOrphanChecker creates a new orphan checker
func NewOrphanChecker(db *sql.DB) *OrphanChecker {
	return &OrphanChecker{db: db}
}

// Check finds transactions without operations
func (c *OrphanChecker) Check(ctx context.Context, limit int) (*domain.OrphanCheckResult, error) {
	result := &domain.OrphanCheckResult{}

	// Summary query
	summaryQuery := `
		SELECT
			COUNT(*) FILTER (WHERE operation_count = 0) as orphan_transactions,
			COUNT(*) FILTER (WHERE operation_count = 1) as partial_transactions
		FROM (
			SELECT t.id, COUNT(o.id) as operation_count
			FROM transaction t
			LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
			WHERE t.deleted_at IS NULL
			  AND t.status NOT IN ('NOTED', 'PENDING')
			GROUP BY t.id
		) sub
	`

	err := c.db.QueryRowContext(ctx, summaryQuery).Scan(
		&result.OrphanTransactions,
		&result.PartialTransactions,
	)
	if err != nil {
		return nil, fmt.Errorf("orphan summary query failed: %w", err)
	}

	// Status
	if result.OrphanTransactions == 0 && result.PartialTransactions == 0 {
		result.Status = "HEALTHY"
	} else if result.OrphanTransactions == 0 {
		result.Status = "WARNING"
	} else {
		result.Status = "CRITICAL"
	}

	// Get detailed orphans
	if (result.OrphanTransactions > 0 || result.PartialTransactions > 0) && limit > 0 {
		detailQuery := `
			SELECT
				t.id as transaction_id,
				t.organization_id::text,
				t.ledger_id::text,
				t.status,
				t.amount,
				t.asset_code,
				t.created_at,
				COUNT(o.id)::int as operation_count
			FROM transaction t
			LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
			WHERE t.deleted_at IS NULL
			  AND t.status NOT IN ('NOTED', 'PENDING')
			GROUP BY t.id, t.organization_id, t.ledger_id, t.status, t.amount, t.asset_code, t.created_at
			HAVING COUNT(o.id) < 2
			ORDER BY t.created_at DESC
			LIMIT $1
		`

		rows, err := c.db.QueryContext(ctx, detailQuery, limit)
		if err != nil {
			return nil, fmt.Errorf("orphan detail query failed: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var o domain.OrphanTransaction
			err := rows.Scan(
				&o.TransactionID, &o.OrganizationID, &o.LedgerID,
				&o.Status, &o.Amount, &o.AssetCode, &o.CreatedAt, &o.OperationCount,
			)
			if err != nil {
				return nil, fmt.Errorf("orphan row scan failed: %w", err)
			}
			result.Orphans = append(result.Orphans, o)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("orphan row iteration failed: %w", err)
		}
	}

	return result, nil
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/adapters/postgres/
```

---

#### Task 12: Referential integrity check
**File**: `components/reconciliation/internal/adapters/postgres/referential_check.go`
**Time**: 5 min
**Agent**: `backend-engineer-golang`

```go
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// ReferentialChecker finds orphan entities
type ReferentialChecker struct {
	onboardingDB  *sql.DB
	transactionDB *sql.DB
}

// NewReferentialChecker creates a new referential checker
func NewReferentialChecker(onboardingDB, transactionDB *sql.DB) *ReferentialChecker {
	return &ReferentialChecker{
		onboardingDB:  onboardingDB,
		transactionDB: transactionDB,
	}
}

// Check finds orphan entities across databases
func (c *ReferentialChecker) Check(ctx context.Context, limit int) (*domain.ReferentialCheckResult, error) {
	result := &domain.ReferentialCheckResult{}

	// Check onboarding DB orphans
	if err := c.checkOnboardingOrphans(ctx, result, limit); err != nil {
		return nil, fmt.Errorf("onboarding orphan check failed: %w", err)
	}

	// Check transaction DB orphans
	if err := c.checkTransactionOrphans(ctx, result, limit); err != nil {
		return nil, fmt.Errorf("transaction orphan check failed: %w", err)
	}

	// Determine status
	total := result.OrphanLedgers + result.OrphanAssets + result.OrphanAccounts +
		result.OrphanOperations + result.OrphanPortfolios
	if total == 0 {
		result.Status = "HEALTHY"
	} else if total < 10 {
		result.Status = "WARNING"
	} else {
		result.Status = "CRITICAL"
	}

	return result, nil
}

func (c *ReferentialChecker) checkOnboardingOrphans(ctx context.Context, result *domain.ReferentialCheckResult, limit int) error {
	query := `
		WITH orphan_ledgers AS (
			SELECT l.id::text as entity_id, 'ledger' as entity_type,
				   'organization' as reference_type, l.organization_id::text as reference_id
			FROM ledger l
			LEFT JOIN organization o ON l.organization_id = o.id AND o.deleted_at IS NULL
			WHERE l.deleted_at IS NULL AND o.id IS NULL
			LIMIT $1
		),
		orphan_assets AS (
			SELECT a.id::text as entity_id, 'asset' as entity_type,
				   'ledger' as reference_type, a.ledger_id::text as reference_id
			FROM asset a
			LEFT JOIN ledger l ON a.ledger_id = l.id AND l.deleted_at IS NULL
			WHERE a.deleted_at IS NULL AND l.id IS NULL
			LIMIT $1
		),
		orphan_accounts AS (
			SELECT a.id::text as entity_id, 'account' as entity_type,
				   'ledger' as reference_type, a.ledger_id::text as reference_id
			FROM account a
			LEFT JOIN ledger l ON a.ledger_id = l.id AND l.deleted_at IS NULL
			WHERE a.deleted_at IS NULL AND l.id IS NULL
			LIMIT $1
		),
		orphan_portfolios AS (
			SELECT p.id::text as entity_id, 'portfolio' as entity_type,
				   'ledger' as reference_type, p.ledger_id::text as reference_id
			FROM portfolio p
			LEFT JOIN ledger l ON p.ledger_id = l.id AND l.deleted_at IS NULL
			WHERE p.deleted_at IS NULL AND l.id IS NULL
			LIMIT $1
		)
		SELECT * FROM orphan_ledgers
		UNION ALL SELECT * FROM orphan_assets
		UNION ALL SELECT * FROM orphan_accounts
		UNION ALL SELECT * FROM orphan_portfolios
	`

	rows, err := c.onboardingDB.QueryContext(ctx, query, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var o domain.OrphanEntity
		if err := rows.Scan(&o.EntityID, &o.EntityType, &o.ReferenceType, &o.ReferenceID); err != nil {
			return err
		}
		result.Orphans = append(result.Orphans, o)

		switch o.EntityType {
		case "ledger":
			result.OrphanLedgers++
		case "asset":
			result.OrphanAssets++
		case "account":
			result.OrphanAccounts++
		case "portfolio":
			result.OrphanPortfolios++
		}
	}

	return rows.Err()
}

func (c *ReferentialChecker) checkTransactionOrphans(ctx context.Context, result *domain.ReferentialCheckResult, limit int) error {
	query := `
		SELECT o.id::text as entity_id, 'operation' as entity_type,
			   'transaction' as reference_type, o.transaction_id::text as reference_id
		FROM operation o
		LEFT JOIN transaction t ON o.transaction_id = t.id AND t.deleted_at IS NULL
		WHERE o.deleted_at IS NULL AND t.id IS NULL
		LIMIT $1
	`

	rows, err := c.transactionDB.QueryContext(ctx, query, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var o domain.OrphanEntity
		if err := rows.Scan(&o.EntityID, &o.EntityType, &o.ReferenceType, &o.ReferenceID); err != nil {
			return err
		}
		result.Orphans = append(result.Orphans, o)
		result.OrphanOperations++
	}

	return rows.Err()
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/adapters/postgres/
```

---

#### Task 13: Sync check (Redis-PostgreSQL version mismatches)
**File**: `components/reconciliation/internal/adapters/postgres/sync_check.go`
**Time**: 4 min
**Agent**: `backend-engineer-golang`

```go
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// SyncChecker detects Redis-PostgreSQL version mismatches
type SyncChecker struct {
	db *sql.DB
}

// NewSyncChecker creates a new sync checker
func NewSyncChecker(db *sql.DB) *SyncChecker {
	return &SyncChecker{db: db}
}

// Check detects version mismatches and stale balances
func (c *SyncChecker) Check(ctx context.Context, staleThresholdSeconds int, limit int) (*domain.SyncCheckResult, error) {
	result := &domain.SyncCheckResult{}

	// Query includes balance_key in join to ensure correct operation matching
	query := `
		WITH balance_ops AS (
			SELECT
				b.id as balance_id,
				a.alias,
				b.asset_code,
				b.version as db_version,
				MAX(o.balance_version_after) as max_op_version,
				EXTRACT(EPOCH FROM (MAX(o.created_at) - b.updated_at))::bigint as staleness_seconds
			FROM balance b
			JOIN account a ON b.account_id = a.id
			LEFT JOIN operation o ON b.account_id = o.account_id
				AND b.asset_code = o.asset_code
				AND b.key = o.balance_key
				AND o.deleted_at IS NULL
				AND o.balance_affected = true
			WHERE b.deleted_at IS NULL
			GROUP BY b.id, a.alias, b.asset_code, b.version, b.updated_at
		)
		SELECT
			balance_id, alias, asset_code, db_version,
			COALESCE(max_op_version, 0)::int as max_op_version,
			COALESCE(staleness_seconds, 0) as staleness_seconds
		FROM balance_ops
		WHERE db_version != COALESCE(max_op_version, 0)
		   OR staleness_seconds > $1
		ORDER BY staleness_seconds DESC NULLS LAST
		LIMIT $2
	`

	rows, err := c.db.QueryContext(ctx, query, staleThresholdSeconds, limit)
	if err != nil {
		return nil, fmt.Errorf("sync check query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s domain.SyncIssue
		if err := rows.Scan(
			&s.BalanceID, &s.Alias, &s.AssetCode, &s.DBVersion,
			&s.MaxOpVersion, &s.StalenessSeconds,
		); err != nil {
			return nil, fmt.Errorf("sync row scan failed: %w", err)
		}
		result.Issues = append(result.Issues, s)

		if s.DBVersion != s.MaxOpVersion {
			result.VersionMismatches++
		}
		if s.StalenessSeconds > int64(staleThresholdSeconds) {
			result.StaleBalances++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sync row iteration failed: %w", err)
	}

	// Status
	total := result.VersionMismatches + result.StaleBalances
	if total == 0 {
		result.Status = "HEALTHY"
	} else if total < 10 {
		result.Status = "WARNING"
	} else {
		result.Status = "CRITICAL"
	}

	return result, nil
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/adapters/postgres/
```

---

#### Task 14: Settlement detector
**File**: `components/reconciliation/internal/adapters/postgres/settlement.go`
**Time**: 4 min
**Agent**: `backend-engineer-golang`

```go
package postgres

import (
	"context"
	"database/sql"
)

// SettlementDetector checks if transactions have fully settled
type SettlementDetector struct {
	db *sql.DB
}

// NewSettlementDetector creates a new settlement detector
func NewSettlementDetector(db *sql.DB) *SettlementDetector {
	return &SettlementDetector{db: db}
}

// GetUnsettledCount returns the count of transactions still processing
// NOTE: Only APPROVED and CANCELED affect balances, NOTED transactions don't need settlement
func (s *SettlementDetector) GetUnsettledCount(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(DISTINCT t.id)
		FROM transaction t
		WHERE t.deleted_at IS NULL
		  AND t.status IN ('APPROVED', 'CANCELED')
		  AND EXISTS (
			  SELECT 1
			  FROM metadata_outbox o
			  WHERE o.entity_id = t.id::text
				AND o.status IN ('PENDING', 'PROCESSING', 'FAILED')
		  )
	`

	var count int
	err := s.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// GetSettledCount returns the count of settled transactions
// NOTE: Only APPROVED and CANCELED affect balances, NOTED transactions don't need settlement
func (s *SettlementDetector) GetSettledCount(ctx context.Context, settlementWaitSeconds int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM transaction t
		WHERE t.deleted_at IS NULL
		  AND t.status IN ('APPROVED', 'CANCELED')
		  AND t.created_at < NOW() - INTERVAL '1 second' * $1
		  AND NOT EXISTS (
			  SELECT 1
			  FROM metadata_outbox o
			  WHERE o.entity_id = t.id::text
				AND o.status IN ('PENDING', 'PROCESSING', 'FAILED')
		  )
	`

	var count int
	err := s.db.QueryRowContext(ctx, query, settlementWaitSeconds).Scan(&count)
	return count, err
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/adapters/postgres/
```

---

### Phase 3 Review Checkpoint
**Severity**: High (core business logic)
**Action**: Full review - verify SQL correctness

---

### Phase 4: Engine & Worker (Tasks 15-18)

#### Task 15: Entity count queries
**File**: `components/reconciliation/internal/adapters/postgres/counts.go`
**Time**: 3 min
**Agent**: `backend-engineer-golang`

```go
package postgres

import (
	"context"
	"database/sql"
)

// EntityCounter counts entities in databases
type EntityCounter struct {
	onboardingDB  *sql.DB
	transactionDB *sql.DB
}

// NewEntityCounter creates a new entity counter
func NewEntityCounter(onboardingDB, transactionDB *sql.DB) *EntityCounter {
	return &EntityCounter{
		onboardingDB:  onboardingDB,
		transactionDB: transactionDB,
	}
}

// OnboardingCounts holds onboarding entity counts
type OnboardingCounts struct {
	Organizations int64
	Ledgers       int64
	Assets        int64
	Accounts      int64
	Portfolios    int64
}

// TransactionCounts holds transaction entity counts
type TransactionCounts struct {
	Transactions int64
	Operations   int64
	Balances     int64
}

// GetOnboardingCounts returns counts from onboarding DB
func (c *EntityCounter) GetOnboardingCounts(ctx context.Context) (*OnboardingCounts, error) {
	query := `
		SELECT
			(SELECT COUNT(*) FROM organization WHERE deleted_at IS NULL) as organizations,
			(SELECT COUNT(*) FROM ledger WHERE deleted_at IS NULL) as ledgers,
			(SELECT COUNT(*) FROM asset WHERE deleted_at IS NULL) as assets,
			(SELECT COUNT(*) FROM account WHERE deleted_at IS NULL) as accounts,
			(SELECT COUNT(*) FROM portfolio WHERE deleted_at IS NULL) as portfolios
	`

	counts := &OnboardingCounts{}
	err := c.onboardingDB.QueryRowContext(ctx, query).Scan(
		&counts.Organizations,
		&counts.Ledgers,
		&counts.Assets,
		&counts.Accounts,
		&counts.Portfolios,
	)

	return counts, err
}

// GetTransactionCounts returns counts from transaction DB
func (c *EntityCounter) GetTransactionCounts(ctx context.Context) (*TransactionCounts, error) {
	query := `
		SELECT
			(SELECT COUNT(*) FROM transaction WHERE deleted_at IS NULL) as transactions,
			(SELECT COUNT(*) FROM operation WHERE deleted_at IS NULL) as operations,
			(SELECT COUNT(*) FROM balance WHERE deleted_at IS NULL) as balances
	`

	counts := &TransactionCounts{}
	err := c.transactionDB.QueryRowContext(ctx, query).Scan(
		&counts.Transactions,
		&counts.Operations,
		&counts.Balances,
	)

	return counts, err
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/adapters/postgres/
```

---

#### Task 16: Reconciliation engine (orchestrator)
**File**: `components/reconciliation/internal/engine/reconciliation.go`
**Time**: 5 min
**Agent**: `backend-engineer-golang`

```go
package engine

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Logger interface
type Logger interface {
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
}

// ReconciliationEngine orchestrates all reconciliation checks
type ReconciliationEngine struct {
	// Checkers
	balanceChecker     *postgres.BalanceChecker
	doubleEntryChecker *postgres.DoubleEntryChecker
	orphanChecker      *postgres.OrphanChecker
	referentialChecker *postgres.ReferentialChecker
	syncChecker        *postgres.SyncChecker
	settlementDetector *postgres.SettlementDetector
	entityCounter      *postgres.EntityCounter

	// MongoDB
	onboardingMongo  *mongo.Database
	transactionMongo *mongo.Database

	// Config
	logger                 Logger
	discrepancyThreshold   int64
	maxDiscrepancies       int
	settlementWaitSeconds  int

	// State
	lastReport *domain.ReconciliationReport
	mu         sync.RWMutex
}

// NewReconciliationEngine creates a new engine
func NewReconciliationEngine(
	onboardingDB, transactionDB *sql.DB,
	onboardingMongo, transactionMongo *mongo.Database,
	logger Logger,
	discrepancyThreshold int64,
	maxDiscrepancies int,
	settlementWaitSeconds int,
) *ReconciliationEngine {
	return &ReconciliationEngine{
		balanceChecker:        postgres.NewBalanceChecker(transactionDB),
		doubleEntryChecker:    postgres.NewDoubleEntryChecker(transactionDB),
		orphanChecker:         postgres.NewOrphanChecker(transactionDB),
		referentialChecker:    postgres.NewReferentialChecker(onboardingDB, transactionDB),
		syncChecker:           postgres.NewSyncChecker(transactionDB),
		settlementDetector:    postgres.NewSettlementDetector(transactionDB),
		entityCounter:         postgres.NewEntityCounter(onboardingDB, transactionDB),
		onboardingMongo:       onboardingMongo,
		transactionMongo:      transactionMongo,
		logger:                logger,
		discrepancyThreshold:  discrepancyThreshold,
		maxDiscrepancies:      maxDiscrepancies,
		settlementWaitSeconds: settlementWaitSeconds,
	}
}

// RunReconciliation executes all reconciliation checks
func (e *ReconciliationEngine) RunReconciliation(ctx context.Context) (*domain.ReconciliationReport, error) {
	startTime := time.Now()
	e.logger.Info("Starting reconciliation run")

	report := &domain.ReconciliationReport{
		Timestamp:    startTime,
		EntityCounts: &domain.EntityCounts{},
	}

	// Get settlement info
	unsettled, err := e.settlementDetector.GetUnsettledCount(ctx)
	if err != nil {
		e.logger.Warnf("Failed to get unsettled count: %v", err)
	} else {
		report.UnsettledTransactions = unsettled
	}

	settled, err := e.settlementDetector.GetSettledCount(ctx, e.settlementWaitSeconds)
	if err != nil {
		e.logger.Warnf("Failed to get settled count: %v", err)
	} else {
		report.SettledTransactions = settled
	}

	// Get entity counts
	e.collectEntityCounts(ctx, report)

	// Run all checks in parallel
	var wg sync.WaitGroup
	wg.Add(5)

	go func() {
		defer wg.Done()
		result, err := e.balanceChecker.Check(ctx, e.discrepancyThreshold, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Balance check failed: %v", err)
			report.BalanceCheck = &domain.BalanceCheckResult{Status: "ERROR"}
		} else {
			report.BalanceCheck = result
		}
	}()

	go func() {
		defer wg.Done()
		result, err := e.doubleEntryChecker.Check(ctx, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Double-entry check failed: %v", err)
			report.DoubleEntryCheck = &domain.DoubleEntryCheckResult{Status: "ERROR"}
		} else {
			report.DoubleEntryCheck = result
		}
	}()

	go func() {
		defer wg.Done()
		result, err := e.orphanChecker.Check(ctx, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Orphan check failed: %v", err)
			report.OrphanCheck = &domain.OrphanCheckResult{Status: "ERROR"}
		} else {
			report.OrphanCheck = result
		}
	}()

	go func() {
		defer wg.Done()
		result, err := e.referentialChecker.Check(ctx, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Referential check failed: %v", err)
			report.ReferentialCheck = &domain.ReferentialCheckResult{Status: "ERROR"}
		} else {
			report.ReferentialCheck = result
		}
	}()

	go func() {
		defer wg.Done()
		result, err := e.syncChecker.Check(ctx, e.settlementWaitSeconds, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Sync check failed: %v", err)
			report.SyncCheck = &domain.SyncCheckResult{Status: "ERROR"}
		} else {
			report.SyncCheck = result
		}
	}()

	wg.Wait()

	// Set defaults for nil checks
	if report.MetadataCheck == nil {
		report.MetadataCheck = &domain.MetadataCheckResult{Status: "SKIPPED"}
	}

	// Determine overall status
	report.DetermineOverallStatus()
	report.Duration = time.Since(startTime).String()

	// Store last report
	e.mu.Lock()
	e.lastReport = report
	e.mu.Unlock()

	e.logger.Infof("Reconciliation complete: status=%s, duration=%s", report.Status, report.Duration)
	return report, nil
}

func (e *ReconciliationEngine) collectEntityCounts(ctx context.Context, report *domain.ReconciliationReport) {
	// Onboarding counts
	onboarding, err := e.entityCounter.GetOnboardingCounts(ctx)
	if err != nil {
		e.logger.Warnf("Failed to get onboarding counts: %v", err)
	} else {
		report.EntityCounts.Organizations = onboarding.Organizations
		report.EntityCounts.Ledgers = onboarding.Ledgers
		report.EntityCounts.Assets = onboarding.Assets
		report.EntityCounts.Accounts = onboarding.Accounts
		report.EntityCounts.Portfolios = onboarding.Portfolios
	}

	// Transaction counts
	transaction, err := e.entityCounter.GetTransactionCounts(ctx)
	if err != nil {
		e.logger.Warnf("Failed to get transaction counts: %v", err)
	} else {
		report.EntityCounts.Transactions = transaction.Transactions
		report.EntityCounts.Operations = transaction.Operations
		report.EntityCounts.Balances = transaction.Balances
	}
}

// GetLastReport returns the most recent reconciliation report
func (e *ReconciliationEngine) GetLastReport() *domain.ReconciliationReport {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastReport
}

// IsHealthy returns true if the last report showed no critical issues
func (e *ReconciliationEngine) IsHealthy() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.lastReport == nil {
		return false
	}
	return e.lastReport.Status != "CRITICAL"
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/engine/
```

---

#### Task 17: Worker implementation
**File**: `components/reconciliation/internal/bootstrap/worker.go`
**Time**: 4 min
**Agent**: `backend-engineer-golang`

```go
package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/engine"
)

// ReconciliationWorker runs reconciliation checks on a schedule
type ReconciliationWorker struct {
	engine  *engine.ReconciliationEngine
	logger  libLog.Logger
	config  *Config
	running atomic.Bool // Guard against concurrent runs
}

// NewReconciliationWorker creates a new reconciliation worker
func NewReconciliationWorker(
	eng *engine.ReconciliationEngine,
	logger libLog.Logger,
	config *Config,
) *ReconciliationWorker {
	return &ReconciliationWorker{
		engine: eng,
		logger: logger,
		config: config,
	}
}

// Run starts the reconciliation worker
func (w *ReconciliationWorker) Run(l *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := w.config.GetReconciliationInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.logger.Infof("Reconciliation worker started, interval: %s", interval)

	// Run initial reconciliation
	w.runReconciliation(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Reconciliation worker shutting down")
			return nil

		case <-ticker.C:
			w.runReconciliation(ctx)
		}
	}
}

// runReconciliation executes a single reconciliation run with concurrency guard
func (w *ReconciliationWorker) runReconciliation(ctx context.Context) {
	// Guard against concurrent runs
	if !w.running.CompareAndSwap(false, true) {
		w.logger.Warnf("Skipping reconciliation - previous run still in progress")
		return
	}
	defer w.running.Store(false)

	report, err := w.engine.RunReconciliation(ctx)
	if err != nil {
		w.logger.Errorf("Reconciliation failed: %v", err)
		return
	}

	// Log summary with nil checks
	balanceTotal, balanceDisc := 0, 0
	txnTotal, txnUnbal := 0, 0

	if report.BalanceCheck != nil {
		balanceTotal = report.BalanceCheck.TotalBalances
		balanceDisc = report.BalanceCheck.BalancesWithDiscrepancy
	}
	if report.DoubleEntryCheck != nil {
		txnTotal = report.DoubleEntryCheck.TotalTransactions
		txnUnbal = report.DoubleEntryCheck.UnbalancedTransactions
	}

	w.logger.Infof(
		"Reconciliation: status=%s, balances=%d (disc=%d), txns=%d (unbal=%d), settled=%d, unsettled=%d",
		report.Status,
		balanceTotal,
		balanceDisc,
		txnTotal,
		txnUnbal,
		report.SettledTransactions,
		report.UnsettledTransactions,
	)
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/bootstrap/
```

---

#### Task 18: HTTP status server
**File**: `components/reconciliation/internal/bootstrap/server.go`
**Time**: 4 min
**Agent**: `backend-engineer-golang`

```go
package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/engine"
)

// HTTPServer provides status endpoints
type HTTPServer struct {
	app     *fiber.App
	address string
	engine  *engine.ReconciliationEngine
	logger  libLog.Logger
}

// NewHTTPServer creates a new HTTP server with security middleware
func NewHTTPServer(
	address string,
	eng *engine.ReconciliationEngine,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	version string,
	envName string,
) *HTTPServer {
	app := fiber.New(fiber.Config{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		BodyLimit:    1 * 1024 * 1024, // 1MB max (endpoints don't need large bodies)
	})

	// Security middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(helmet.New())

	server := &HTTPServer{
		app:     app,
		address: address,
		engine:  eng,
		logger:  logger,
	}

	// Public health endpoints (no auth required)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	app.Get("/version", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"version": version, "env": envName})
	})

	// Reconciliation endpoints
	// Note: For production, add authentication middleware here:
	// app.Use("/reconciliation", auth.Authorize("reconciliation", "status", "get"))
	app.Get("/reconciliation/status", server.getStatus)
	app.Get("/reconciliation/report", server.getReport)

	// Rate-limited manual trigger endpoint
	// Global limit: 1 request per minute to prevent DoS
	app.Post("/reconciliation/run",
		limiter.New(limiter.Config{
			Max:        1,
			Expiration: 60 * time.Second,
			KeyGenerator: func(c *fiber.Ctx) string {
				return "global" // Global rate limit, not per-IP
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{
					"error": "Rate limit exceeded. Reconciliation can only be triggered once per minute.",
				})
			},
		}),
		server.triggerRun,
	)

	return server
}

// Run starts the HTTP server
func (s *HTTPServer) Run(l *libCommons.Launcher) error {
	s.logger.Infof("HTTP server starting on %s", s.address)
	return s.app.Listen(s.address)
}

func (s *HTTPServer) getStatus(c *fiber.Ctx) error {
	report := s.engine.GetLastReport()
	if report == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"status":  "UNKNOWN",
			"message": "No reconciliation run completed yet",
		})
	}

	statusCode := http.StatusOK
	if report.Status == "CRITICAL" {
		statusCode = http.StatusServiceUnavailable
	}

	// Build checks map with nil safety
	checks := fiber.Map{}
	if report.BalanceCheck != nil {
		checks["balance"] = report.BalanceCheck.Status
	} else {
		checks["balance"] = "UNKNOWN"
	}
	if report.DoubleEntryCheck != nil {
		checks["double_entry"] = report.DoubleEntryCheck.Status
	} else {
		checks["double_entry"] = "UNKNOWN"
	}
	if report.ReferentialCheck != nil {
		checks["referential"] = report.ReferentialCheck.Status
	} else {
		checks["referential"] = "UNKNOWN"
	}
	if report.SyncCheck != nil {
		checks["sync"] = report.SyncCheck.Status
	} else {
		checks["sync"] = "UNKNOWN"
	}
	if report.OrphanCheck != nil {
		checks["orphans"] = report.OrphanCheck.Status
	} else {
		checks["orphans"] = "UNKNOWN"
	}
	if report.MetadataCheck != nil {
		checks["metadata"] = report.MetadataCheck.Status
	} else {
		checks["metadata"] = "UNKNOWN"
	}

	return c.Status(statusCode).JSON(fiber.Map{
		"status":    report.Status,
		"timestamp": report.Timestamp,
		"duration":  report.Duration,
		"checks":    checks,
	})
}

func (s *HTTPServer) getReport(c *fiber.Ctx) error {
	report := s.engine.GetLastReport()
	if report == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "No reconciliation report available",
		})
	}
	return c.JSON(report)
}

func (s *HTTPServer) triggerRun(c *fiber.Ctx) error {
	requestID := c.Locals("requestid")
	s.logger.Infof("Manual reconciliation triggered via API (request_id=%v)", requestID)

	// Add timeout for HTTP-triggered runs
	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Minute)
	defer cancel()

	report, err := s.engine.RunReconciliation(ctx)
	if err != nil {
		// Log full error internally
		s.logger.Errorf("Reconciliation failed (request_id=%v): %v", requestID, err)

		// Return sanitized error to client
		if errors.Is(err, context.DeadlineExceeded) {
			return c.Status(http.StatusGatewayTimeout).JSON(fiber.Map{
				"error":      "Reconciliation timed out",
				"request_id": requestID,
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error":      "Reconciliation failed. Check server logs for details.",
			"request_id": requestID,
		})
	}

	return c.JSON(report)
}
```

**Verification**:
```bash
go build ./components/reconciliation/internal/bootstrap/
```

---

### Phase 4 Review Checkpoint
**Severity**: High
**Action**: Full review of engine orchestration logic

---

### Phase 5: Docker & Infrastructure (Tasks 19-23)

#### Task 19: Dockerfile
**File**: `components/reconciliation/Dockerfile`
**Time**: 2 min
**Agent**: `devops-engineer`

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETPLATFORM
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) \
    go build -tags netgo -ldflags '-s -w' \
    -o /reconciliation ./components/reconciliation/cmd/app/main.go

FROM gcr.io/distroless/static-debian12
COPY --from=builder /reconciliation /reconciliation
EXPOSE 3005
ENTRYPOINT ["/reconciliation"]
```

---

#### Task 20: docker-compose.yml
**File**: `components/reconciliation/docker-compose.yml`
**Time**: 2 min
**Agent**: `devops-engineer`

```yaml
services:
  reconciliation:
    container_name: midaz-reconciliation
    restart: always
    build:
      context: ../../
      dockerfile: ./components/reconciliation/Dockerfile
    env_file:
      - .env
    ports:
      - "${SERVER_PORT:-3005}:${SERVER_PORT:-3005}"
    networks:
      - infra_infra-network
    depends_on:
      - midaz-postgres-replica
      - midaz-mongodb

networks:
  infra_infra-network:
    external: true
```

---

#### Task 21: Makefile
**File**: `components/reconciliation/Makefile`
**Time**: 3 min
**Agent**: `devops-engineer`

```makefile
SERVICE_NAME := reconciliation
BIN_DIR := ./.bin

.PHONY: help build test clean up down logs status report trigger

help:
	@echo "Reconciliation Worker Commands"
	@echo "  build    - Build binary"
	@echo "  test     - Run tests"
	@echo "  up       - Start container"
	@echo "  down     - Stop container"
	@echo "  logs     - View logs"
	@echo "  status   - Get reconciliation status"
	@echo "  report   - Get full report"
	@echo "  trigger  - Trigger manual run"

build:
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/$(SERVICE_NAME) ./cmd/app/main.go

test:
	@go test -v -race ./...

clean:
	@rm -rf $(BIN_DIR)

up:
	@docker-compose up -d

down:
	@docker-compose down

logs:
	@docker-compose logs -f

status:
	@curl -s http://localhost:3005/reconciliation/status | jq .

report:
	@curl -s http://localhost:3005/reconciliation/report | jq .

trigger:
	@curl -s -X POST http://localhost:3005/reconciliation/run | jq .
```

---

#### Task 22: README.md
**File**: `components/reconciliation/README.md`
**Time**: 2 min
**Agent**: `devops-engineer`

```markdown
# Reconciliation Worker

Continuous database consistency monitoring for Midaz.

## Quick Start

```bash
# Copy environment config
cp .env.example .env

# Start (requires infra running)
make up

# Check status
make status

# View full report
make report
```

## Checks Performed

| Check | Purpose | Severity |
|-------|---------|----------|
| Balance Consistency | balance = Σcredits - Σdebits | WARNING/CRITICAL |
| Double-Entry | credits = debits per transaction | CRITICAL |
| Referential Integrity | No orphan entities | WARNING/CRITICAL |
| Sync (Redis-PG) | Version consistency | WARNING |
| Orphan Transactions | Transactions with operations | CRITICAL |

## API Endpoints

- `GET /health` - Liveness probe
- `GET /reconciliation/status` - Quick status
- `GET /reconciliation/report` - Full report
- `POST /reconciliation/run` - Manual trigger

## Configuration

See `.env.example` for all options.

Key settings:
- `RECONCILIATION_INTERVAL_SECONDS` - Check interval (default: 300)
- `SETTLEMENT_WAIT_SECONDS` - Wait for async settlement (default: 300)
- `DISCREPANCY_THRESHOLD` - Min discrepancy to report (default: 0)
```

---

#### Task 23: Update root Makefile (optional)
**File**: `Makefile` (append)
**Time**: 2 min
**Agent**: `devops-engineer`

```makefile
# Add to existing Makefile

.PHONY: reconciliation-up reconciliation-down reconciliation-status

reconciliation-up:
	@$(MAKE) -C components/reconciliation up

reconciliation-down:
	@$(MAKE) -C components/reconciliation down

reconciliation-status:
	@$(MAKE) -C components/reconciliation status
```

---

### Final Verification

```bash
# Full build from repo root
go build ./components/reconciliation/...

# Verify isolation - should show ONLY these imports:
grep -r "LerianStudio/midaz" components/reconciliation/ --include="*.go" | grep -v "components/reconciliation"
# Expected: NO OUTPUT (no imports from other components)

# Check external deps only
grep -r "LerianStudio" components/reconciliation/ --include="*.go" | head -20
# Expected: Only lib-commons and components/reconciliation
```

---

## Isolation Checklist

- [x] No imports from `pkg/`
- [x] No imports from `components/transaction/`
- [x] No imports from `components/onboarding/`
- [x] Self-contained `safego` package
- [x] Direct SQL queries (no ORM coupling)
- [x] Direct `database/sql` connections
- [x] Only `lib-commons` external dependency
- [x] Self-contained Dockerfile
- [x] Self-contained docker-compose
- [x] Self-contained Makefile

**Result**: `components/reconciliation/` can be cherry-picked as a single unit.
