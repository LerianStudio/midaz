// Package balance provides PostgreSQL data models and mapping for account balances.
//
// This package implements the infrastructure layer for balance persistence, providing:
//   - BalancePostgreSQLModel: Database model with SQL-specific types
//   - Entity conversion: Bidirectional mapping between domain and persistence models
//   - Soft delete support: DeletedAt field for logical deletion
//
// Architecture Context:
// In hexagonal architecture, this package is an adapter that bridges the domain model
// (mmodel.Balance) to the PostgreSQL database. The separation ensures:
//   - Domain models remain database-agnostic
//   - SQL-specific types (sql.NullTime) are isolated here
//   - Version control enables optimistic locking for concurrent updates
//
// Balance Concept:
// A balance represents the current state of funds for an account-asset pair:
//   - Available: Funds ready for immediate use
//   - OnHold: Funds reserved but not yet committed (pending transactions)
//   - Total = Available + OnHold
//
// Multi-Balance Support:
// Accounts can have multiple balances differentiated by the Key field:
//   - "default": Primary balance (created automatically with account)
//   - Custom keys: Secondary balances for specific purposes (e.g., "savings", "escrow")
//
// Related Packages:
//   - mmodel: Domain models for Balance entity
//   - transaction: Uses balances for debit/credit operations
//   - account: Owns balances (one-to-many relationship)
package balance

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// BalancePostgreSQLModel represents a balance entity in the PostgreSQL database.
//
// This model maps directly to the "balance" table and handles SQL-specific type
// conversions. It uses sql.NullTime for nullable timestamp columns and decimal.Decimal
// for precise monetary calculations.
//
// Table Schema:
//
//	balance (
//	    id             VARCHAR PRIMARY KEY,
//	    organization_id VARCHAR NOT NULL,  -- Multi-tenant isolation
//	    ledger_id      VARCHAR NOT NULL,   -- Ledger scope
//	    account_id     VARCHAR NOT NULL,   -- Parent account
//	    alias          VARCHAR NOT NULL,   -- Human-readable identifier
//	    key            VARCHAR NOT NULL,   -- Balance key (default, savings, etc.)
//	    asset_code     VARCHAR NOT NULL,   -- Currency/asset (USD, BRL, BTC)
//	    available      DECIMAL NOT NULL,   -- Immediately usable funds
//	    on_hold        DECIMAL NOT NULL,   -- Reserved funds
//	    version        BIGINT NOT NULL,    -- Optimistic locking version
//	    account_type   VARCHAR NOT NULL,   -- Account type (deposit, credit, etc.)
//	    allow_sending  BOOLEAN NOT NULL,   -- Can debit from this balance
//	    allow_receiving BOOLEAN NOT NULL,  -- Can credit to this balance
//	    created_at     TIMESTAMP NOT NULL,
//	    updated_at     TIMESTAMP NOT NULL,
//	    deleted_at     TIMESTAMP           -- NULL = active, set = soft deleted
//	)
//
// Indexes:
//   - PRIMARY KEY on id
//   - UNIQUE (organization_id, ledger_id, account_id, key) for balance lookup
//   - INDEX on (organization_id, ledger_id, alias) for alias-based queries
//
// Concurrency:
// The Version field enables optimistic locking. Update operations include
// "WHERE version < new_version" to prevent lost updates in concurrent scenarios.
type BalancePostgreSQLModel struct {
	ID             string          // UUIDv7 primary key
	OrganizationID string          // Tenant isolation
	LedgerID       string          // Ledger scope
	AccountID      string          // Parent account reference
	Alias          string          // Human-readable account alias
	Key            string          // Balance key within account (default: "default")
	AssetCode      string          // ISO 4217 currency or custom asset code
	Available      decimal.Decimal // Funds available for transactions
	OnHold         decimal.Decimal // Funds reserved (pending transactions)
	Version        int64           // Optimistic locking version counter
	AccountType    string          // deposit, credit, equity, liability, etc.
	AllowSending   bool            // Whether debits are allowed
	AllowReceiving bool            // Whether credits are allowed
	CreatedAt      time.Time       // Record creation timestamp
	UpdatedAt      time.Time       // Last modification timestamp
	DeletedAt      sql.NullTime    // Soft delete timestamp (NULL = active)
}

// FromEntity converts a domain Balance entity to the PostgreSQL model.
//
// This method maps domain model fields to database columns, handling:
//   - Direct field mapping for most fields
//   - Key defaulting: If balance.Key is nil/empty, defaults to "default"
//   - DeletedAt conversion: Converts *time.Time to sql.NullTime
//
// The receiver is fully overwritten, so any previous state is discarded.
//
// Parameters:
//   - balance: Domain model from the application layer
//
// Key Handling:
// The Key field identifies balances within an account. When not specified,
// it defaults to "default" to ensure every account has a primary balance.
// This enables multi-balance scenarios like:
//   - account.Key("default") -> main operating balance
//   - account.Key("savings") -> earmarked savings
//   - account.Key("escrow") -> funds held in escrow
func (b *BalancePostgreSQLModel) FromEntity(balance *mmodel.Balance) {
	*b = BalancePostgreSQLModel{
		ID:             balance.ID,
		OrganizationID: balance.OrganizationID,
		LedgerID:       balance.LedgerID,
		AccountID:      balance.AccountID,
		Alias:          balance.Alias,
		AssetCode:      balance.AssetCode,
		Available:      balance.Available,
		OnHold:         balance.OnHold,
		Version:        balance.Version,
		AccountType:    balance.AccountType,
		AllowSending:   balance.AllowSending,
		AllowReceiving: balance.AllowReceiving,
		CreatedAt:      balance.CreatedAt,
		UpdatedAt:      balance.UpdatedAt,
	}

	if libCommons.IsNilOrEmpty(&balance.Key) {
		b.Key = "default"
	} else {
		b.Key = balance.Key
	}

	if balance.DeletedAt != nil {
		deletedAtCopy := *balance.DeletedAt
		b.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}

// ToEntity converts the PostgreSQL model to a domain Balance entity.
//
// This method maps database columns back to domain model fields for use
// in the application layer. The conversion is straightforward with no
// special handling required (DeletedAt is not exposed to the domain layer
// as soft-deleted records are filtered at the query level).
//
// Returns:
//   - *mmodel.Balance: Domain model ready for business logic processing
//
// Note: The returned balance is a new allocation. The caller owns the
// returned pointer and can safely modify it without affecting the
// database model.
func (b *BalancePostgreSQLModel) ToEntity() *mmodel.Balance {
	balance := &mmodel.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		Key:            b.Key,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Version:        b.Version,
		AccountType:    b.AccountType,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}

	return balance
}
