# Database Guide

## Database Stack

Midaz uses a **polyglot persistence** approach with multiple databases optimized for different use cases:

- **PostgreSQL 17** (with replica): Primary transactional data, relational integrity
- **MongoDB 8** (replica set): Metadata, flexible schemas, document storage
- **Valkey 8**: Caching layer (Redis-compatible fork)

## PostgreSQL Patterns

### Primary Database: Transactional Data

**Use For**:
- Accounts, Ledgers, Transactions, Balances
- Any data requiring ACID guarantees
- Complex queries with joins
- Financial data requiring strong consistency

### Schema Location

Migrations are typically stored in:
```
components/{service}/migrations/
├── 000001_initial_schema.up.sql
├── 000001_initial_schema.down.sql
├── 000002_add_accounts.up.sql
└── 000002_add_accounts.down.sql
```

### Migration Patterns

**Always follow these rules**:

1. **Use TIMESTAMPTZ, never TIMESTAMP**
   ```sql
   -- ❌ BAD
   created_at TIMESTAMP

   -- ✅ GOOD
   created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
   ```

2. **Include IF NOT EXISTS for idempotency**
   ```sql
   CREATE TABLE IF NOT EXISTS accounts (
       id UUID PRIMARY KEY,
       name VARCHAR(255) NOT NULL,
       ...
   );

   CREATE INDEX IF NOT EXISTS idx_accounts_ledger_id ON accounts(ledger_id);
   ```

3. **Every .up.sql must have matching .down.sql**
   ```sql
   -- 000003_add_portfolio.up.sql
   CREATE TABLE portfolios (...);

   -- 000003_add_portfolio.down.sql
   DROP TABLE IF EXISTS portfolios CASCADE;
   ```

4. **Use UUIDs for primary keys**
   ```sql
   CREATE TABLE accounts (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       ...
   );
   ```

5. **Add constraints**
   ```sql
   CREATE TABLE accounts (
       id UUID PRIMARY KEY,
       balance DECIMAL(20, 8) NOT NULL CHECK (balance >= 0),
       type VARCHAR(50) NOT NULL CHECK (type IN ('DEPOSIT', 'SAVINGS', 'INVESTMENT')),
       ...
   );
   ```

### Repository Implementation Pattern

**Interface** (Port)
```go
// components/onboarding/internal/adapters/postgres/account/account.go

type Repository interface {
    Create(ctx context.Context, account *mmodel.Account) error
    Find(ctx context.Context, orgID, ledgerID, accountID uuid.UUID) (*mmodel.Account, error)
    FindAll(ctx context.Context, orgID, ledgerID uuid.UUID, filter Filter) (*Pagination, error)
    Update(ctx context.Context, account *mmodel.Account) error
    Delete(ctx context.Context, orgID, ledgerID, accountID uuid.UUID) error
}
```

**Implementation**
```go
// components/onboarding/internal/adapters/postgres/account/account.postgresql.go

type PostgresRepository struct {
    db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
    assert.NotNil(db, "database connection")
    return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, account *mmodel.Account) error {
    query := `
        INSERT INTO accounts (id, name, type, organization_id, ledger_id, balance, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    `

    _, err := r.db.ExecContext(ctx, query,
        account.ID,
        account.Name,
        account.Type,
        account.OrganizationID,
        account.LedgerID,
        account.Balance,
        account.CreatedAt,
        account.UpdatedAt,
    )
    if err != nil {
        // Handle PostgreSQL-specific errors
        if pqErr, ok := err.(*pq.Error); ok {
            if pqErr.Code == "23505" { // unique_violation
                return pkg.EntityConflictError{
                    EntityType: "Account",
                    Field:      extractConstraintField(pqErr),
                    Value:      account.Name,
                    Err:        err,
                }
            }
        }
        return fmt.Errorf("executing insert: %w", err)
    }

    return nil
}

func (r *PostgresRepository) Find(ctx context.Context, orgID, ledgerID, accountID uuid.UUID) (*mmodel.Account, error) {
    query := `
        SELECT id, name, type, organization_id, ledger_id, balance, created_at, updated_at
        FROM accounts
        WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL
    `

    var account mmodel.Account
    err := r.db.GetContext(ctx, &account, query, orgID, ledgerID, accountID)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, nil  // Not found, not an error
        }
        return nil, fmt.Errorf("querying account: %w", err)
    }

    return &account, nil
}
```

### PostgreSQL Error Handling

```go
import "github.com/lib/pq"

// Common PostgreSQL error codes
const (
    UniqueViolation      = "23505"
    ForeignKeyViolation  = "23503"
    CheckViolation       = "23514"
    NotNullViolation     = "23502"
)

func handlePostgresError(err error) error {
    pqErr, ok := err.(*pq.Error)
    if !ok {
        return err
    }

    switch pqErr.Code {
    case UniqueViolation:
        return pkg.EntityConflictError{...}
    case ForeignKeyViolation:
        return pkg.ValidationError{Code: constant.ErrInvalidForeignKey}
    case CheckViolation:
        return pkg.ValidationError{Code: constant.ErrConstraintViolation}
    case NotNullViolation:
        return pkg.ValidationError{Code: constant.ErrRequiredField}
    default:
        return err
    }
}
```

### Transaction Patterns

```go
func (r *PostgresRepository) TransferBalance(ctx context.Context, from, to uuid.UUID, amount decimal.Decimal) error {
    // Start transaction
    tx, err := r.db.BeginTxx(ctx, nil)
    if err != nil {
        return fmt.Errorf("starting transaction: %w", err)
    }
    defer tx.Rollback()  // Safe to call even after Commit

    // Debit source account
    _, err = tx.ExecContext(ctx, `
        UPDATE accounts
        SET balance = balance - $1, updated_at = CURRENT_TIMESTAMP
        WHERE id = $2 AND balance >= $1
    `, amount, from)
    if err != nil {
        return fmt.Errorf("debiting source account: %w", err)
    }

    // Credit destination account
    _, err = tx.ExecContext(ctx, `
        UPDATE accounts
        SET balance = balance + $1, updated_at = CURRENT_TIMESTAMP
        WHERE id = $2
    `, amount, to)
    if err != nil {
        return fmt.Errorf("crediting destination account: %w", err)
    }

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("committing transaction: %w", err)
    }

    return nil
}
```

## MongoDB Patterns

### Use Cases for MongoDB

**Use For**:
- Metadata (flexible schemas)
- Audit logs
- Configuration data
- Historical snapshots

**Don't Use For**:
- Financial transaction data (use PostgreSQL)
- Data requiring strong consistency guarantees
- Complex relational queries

### Repository Pattern for MongoDB

```go
type MongoRepository struct {
    collection *mongo.Collection
}

func NewMongoRepository(db *mongo.Database) *MongoRepository {
    return &MongoRepository{
        collection: db.Collection("metadata"),
    }
}

func (r *MongoRepository) Save(ctx context.Context, metadata *Metadata) error {
    _, err := r.collection.InsertOne(ctx, metadata)
    if err != nil {
        if mongo.IsDuplicateKeyError(err) {
            return pkg.EntityConflictError{...}
        }
        return fmt.Errorf("inserting metadata: %w", err)
    }
    return nil
}

func (r *MongoRepository) Find(ctx context.Context, id string) (*Metadata, error) {
    filter := bson.M{"_id": id}

    var metadata Metadata
    err := r.collection.FindOne(ctx, filter).Decode(&metadata)
    if err != nil {
        if err == mongo.ErrNoDocuments {
            return nil, nil  // Not found
        }
        return nil, fmt.Errorf("querying metadata: %w", err)
    }

    return &metadata, nil
}
```

## Valkey (Redis) Caching

### Cache Patterns

**Cache-Aside Pattern**
```go
func (s *Service) GetAccount(ctx context.Context, id uuid.UUID) (*Account, error) {
    // 1. Try cache first
    cacheKey := fmt.Sprintf("account:%s", id)
    cached, err := s.cache.Get(ctx, cacheKey)
    if err == nil && cached != nil {
        return parseAccount(cached), nil
    }

    // 2. Cache miss - query database
    account, err := s.repo.Find(ctx, id)
    if err != nil {
        return nil, err
    }

    // 3. Populate cache
    if account != nil {
        s.cache.Set(ctx, cacheKey, account, 5*time.Minute)
    }

    return account, nil
}
```

**Cache Invalidation**
```go
func (s *Service) UpdateAccount(ctx context.Context, account *Account) error {
    // 1. Update database
    if err := s.repo.Update(ctx, account); err != nil {
        return err
    }

    // 2. Invalidate cache
    cacheKey := fmt.Sprintf("account:%s", account.ID)
    s.cache.Delete(ctx, cacheKey)

    return nil
}
```

### Distributed Lock Pattern

```go
import "github.com/go-redsync/redsync/v4"

func (s *Service) ProcessWithLock(ctx context.Context, resourceID string) error {
    // Acquire distributed lock
    lockKey := fmt.Sprintf("lock:process:%s", resourceID)
    mutex := s.redsync.NewMutex(lockKey, redsync.WithExpiry(30*time.Second))

    if err := mutex.LockContext(ctx); err != nil {
        return fmt.Errorf("acquiring lock: %w", err)
    }
    defer mutex.UnlockContext(ctx)

    // Process with exclusive access
    return s.doWork(ctx, resourceID)
}
```

## Connection Management

### PostgreSQL Connection Pool

```go
func NewPostgresConnection(config Config) (*sqlx.DB, error) {
    dsn := fmt.Sprintf(
        "host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        config.Host, config.Port, config.User, config.Password,
        config.Database, config.SSLMode,
    )

    db, err := sqlx.Connect("postgres", dsn)
    if err != nil {
        return nil, fmt.Errorf("connecting to PostgreSQL: %w", err)
    }

    // Connection pool settings
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)
    db.SetConnMaxIdleTime(1 * time.Minute)

    // Verify connection
    if err := db.Ping(); err != nil {
        return nil, fmt.Errorf("pinging PostgreSQL: %w", err)
    }

    return db, nil
}
```

### MongoDB Connection

```go
func NewMongoConnection(config Config) (*mongo.Client, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    uri := fmt.Sprintf("mongodb://%s:%s@%s:%d/%s?replicaSet=%s",
        config.User, config.Password, config.Host, config.Port,
        config.Database, config.ReplicaSet,
    )

    clientOptions := options.Client().
        ApplyURI(uri).
        SetMaxPoolSize(100).
        SetMinPoolSize(10)

    client, err := mongo.Connect(ctx, clientOptions)
    if err != nil {
        return nil, fmt.Errorf("connecting to MongoDB: %w", err)
    }

    // Verify connection
    if err := client.Ping(ctx, nil); err != nil {
        return nil, fmt.Errorf("pinging MongoDB: %w", err)
    }

    return client, nil
}
```

## Database Testing

### Using sqlmock for Unit Tests

```go
func TestRepository_Create(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    sqlxDB := sqlx.NewDb(db, "postgres")
    repo := NewPostgresRepository(sqlxDB)

    account := &mmodel.Account{
        ID:   uuid.New(),
        Name: "Test Account",
    }

    mock.ExpectExec(`INSERT INTO accounts`).
        WithArgs(account.ID, account.Name).
        WillReturnResult(sqlmock.NewResult(1, 1))

    err = repo.Create(context.Background(), account)

    assert.NoError(t, err)
    assert.NoError(t, mock.ExpectationsWereMet())
}
```

### Integration Tests with Real Database

```go
func TestRepository_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Connect to test database
    db := setupTestDatabase(t)
    defer teardownTestDatabase(t, db)

    repo := NewPostgresRepository(db)

    // Create account
    account := &mmodel.Account{...}
    err := repo.Create(context.Background(), account)
    require.NoError(t, err)

    // Verify account exists
    found, err := repo.Find(context.Background(), account.ID)
    require.NoError(t, err)
    assert.Equal(t, account.Name, found.Name)
}
```

## Database Checklist

✅ **Use TIMESTAMPTZ** for all timestamp columns

✅ **Include IF NOT EXISTS** in migrations for idempotency

✅ **Create matching .up.sql and .down.sql** files

✅ **Use UUIDs** for primary keys

✅ **Add database constraints** (CHECK, FOREIGN KEY, UNIQUE)

✅ **Handle PostgreSQL-specific errors** with pq error codes

✅ **Return nil entity + nil error** for "not found" cases

✅ **Use transactions** for multi-step operations

✅ **Configure connection pools** appropriately

✅ **Test with sqlmock** for unit tests

✅ **Use real databases** for integration tests

## Related Documentation

- Architecture: `docs/agents/architecture.md`
- Error Handling: `docs/agents/error-handling.md`
- Testing: `docs/agents/testing.md`
