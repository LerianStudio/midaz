package account

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Repository provides an interface for account persistence operations.
//
// This interface defines the contract for account CRUD operations, following
// the repository pattern from Domain-Driven Design. It abstracts PostgreSQL-specific
// implementation details from the application layer.
//
// Design Decisions:
//
//   - Multi-tenant scoping: All operations require organizationID and ledgerID
//   - Optional portfolio: Many operations accept optional portfolioID for sub-scoping
//   - Soft delete: Delete marks records, FindWithDeleted retrieves them
//   - Alias support: Human-readable alternative to UUIDs
//   - Batch operations: ListByIDs, ListByAlias for efficient bulk lookups
//
// Usage:
//
//	repo := account.NewAccountPostgreSQLRepository(connection)
//	acc, err := repo.Create(ctx, &account)
//	found, err := repo.Find(ctx, orgID, ledgerID, nil, accountID)
//
// Thread Safety:
//
// All methods are thread-safe. The underlying database driver handles connection
// pooling and concurrent access.
//
// Observability:
//
// All methods create OpenTelemetry spans for distributed tracing.
// Span names follow the pattern: postgres.<operation>_account
type Repository interface {
	Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.Pagination) ([]*mmodel.Account, error)
	Find(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error)
	FindWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error)
	FindAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string) (*mmodel.Account, error)
	FindByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (bool, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error)
	ListByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias []string) ([]*mmodel.Account, error)
	Update(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error)
	Delete(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error
	ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error)
	ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error)
	Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error)
}

// AccountPostgreSQLRepository is the PostgreSQL implementation of the Repository interface.
//
// This repository provides account persistence using PostgreSQL as the backing store.
// It implements the hexagonal architecture pattern by adapting the domain Repository
// interface to PostgreSQL-specific operations using Squirrel query builder.
//
// Connection Management:
//
// The repository uses a shared PostgresConnection from lib-commons which provides:
//   - Connection pooling
//   - Automatic reconnection
//   - Health checks
//
// Query Building:
//
// Uses Squirrel for type-safe SQL query construction:
//   - Prevents SQL injection
//   - Handles parameter placeholders ($1, $2, etc.)
//   - Supports dynamic WHERE clause building
//
// Lifecycle:
//
//	conn := libPostgres.NewPostgresConnection(cfg)
//	repo := account.NewAccountPostgreSQLRepository(conn)
//	// Use repository...
//	// Connection cleanup handled by PostgresConnection
//
// Thread Safety:
//
// AccountPostgreSQLRepository is thread-safe after initialization.
//
// Fields:
//   - connection: Shared PostgreSQL connection (manages pool and lifecycle)
//   - tableName: Database table name ("account")
type AccountPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewAccountPostgreSQLRepository creates a new AccountPostgreSQLRepository instance.
//
// This constructor initializes the repository with a PostgreSQL connection and
// validates connectivity before returning. It panics on connection failure
// to fail fast during application startup.
//
// Initialization Process:
//  1. Store connection reference
//  2. Set table name to "account"
//  3. Verify connectivity by calling GetDB
//  4. Panic if connection fails (fail-fast startup)
//
// Parameters:
//   - pc: Configured PostgreSQL connection from lib-commons
//
// Returns:
//   - *AccountPostgreSQLRepository: Initialized repository ready for use
//
// Panics:
//   - "Failed to connect database": Connection verification failed
//
// Why Panic on Failure:
//
// This is intentional fail-fast behavior. If PostgreSQL is unavailable at startup,
// the application cannot function correctly. Panicking here ensures:
//   - Clear failure mode during deployment
//   - No silent degradation
//   - Immediate alerting in orchestration systems
func NewAccountPostgreSQLRepository(pc *libPostgres.PostgresConnection) *AccountPostgreSQLRepository {
	c := &AccountPostgreSQLRepository{
		connection: pc,
		tableName:  "account",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create inserts a new account entity into PostgreSQL.
//
// This method persists a new account with full validation and constraint checking.
// It uses UUID v7 for time-ordered, globally unique identifiers.
//
// Operation Process:
//  1. Start OpenTelemetry span for tracing
//  2. Get database connection from pool
//  3. Convert domain model to PostgreSQL model
//  4. Build INSERT query with Squirrel
//  5. Execute query and validate rows affected
//
// Parameters:
//   - ctx: Context with tracing and logging (must not be nil)
//   - acc: Account domain model to persist
//
// Returns:
//   - *mmodel.Account: Created account with generated ID
//   - error: Validation, constraint, or database error
//
// Error Scenarios:
//   - ErrEntityNotFound: Insert affected 0 rows (unexpected)
//   - ErrDuplicateAlias: Alias already exists in ledger
//   - ErrForeignKeyViolation: Invalid organization/ledger/portfolio reference
//   - Connection errors
func (r *AccountPostgreSQLRepository) Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &AccountPostgreSQLModel{}
	record.FromEntity(acc)

	builder := squirrel.Insert(r.tableName).
		Columns(
			"id",
			"name",
			"parent_account_id",
			"entity_id",
			"asset_code",
			"organization_id",
			"ledger_id",
			"portfolio_id",
			"segment_id",
			"status",
			"status_description",
			"alias",
			"type",
			"created_at",
			"updated_at",
			"deleted_at",
			"blocked",
		).
		Values(
			record.ID,
			record.Name,
			record.ParentAccountID,
			record.EntityID,
			record.AssetCode,
			record.OrganizationID,
			record.LedgerID,
			record.PortfolioID,
			record.SegmentID,
			record.Status,
			record.StatusDescription,
			record.Alias,
			record.Type,
			record.CreatedAt,
			record.UpdatedAt,
			record.DeletedAt,
			record.Blocked,
		).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute query", err)

			logger.Errorf("Failed to execute query: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)

		logger.Warnf("Failed to create account: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindAll retrieves an Account entities from the database (including soft-deleted ones) with pagination.
func (r *AccountPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.Pagination) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_accounts")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID))

	if portfolioID != nil && *portfolioID != uuid.Nil {
		findAll = findAll.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	findAll = findAll.OrderBy("created_at " + strings.ToUpper(filter.SortOrder)).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)})

	findAll = findAll.Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return accounts, nil
}

// Find retrieves an Account entity from the database using the provided ID.
func (r *AccountPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	builder := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		builder = builder.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	acc := &AccountPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&acc.ID,
		&acc.Name,
		&acc.ParentAccountID,
		&acc.EntityID,
		&acc.AssetCode,
		&acc.OrganizationID,
		&acc.LedgerID,
		&acc.PortfolioID,
		&acc.SegmentID,
		&acc.Status,
		&acc.StatusDescription,
		&acc.Alias,
		&acc.Type,
		&acc.CreatedAt,
		&acc.UpdatedAt,
		&acc.DeletedAt,
		&acc.Blocked,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			logger.Warnf("Failed to scan row: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
	}

	return acc.ToEntity(), nil
}

// FindWithDeleted retrieves an Account entity from the database using the provided ID (including soft-deleted ones).
func (r *AccountPostgreSQLRepository) FindWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_with_deleted_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	builder := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		builder = builder.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	acc := &AccountPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_with_deleted.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&acc.ID,
		&acc.Name,
		&acc.ParentAccountID,
		&acc.EntityID,
		&acc.AssetCode,
		&acc.OrganizationID,
		&acc.LedgerID,
		&acc.PortfolioID,
		&acc.SegmentID,
		&acc.Status,
		&acc.StatusDescription,
		&acc.Alias,
		&acc.Type,
		&acc.CreatedAt,
		&acc.UpdatedAt,
		&acc.DeletedAt,
		&acc.Blocked,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			logger.Warnf("Failed to scan row: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
	}

	return acc.ToEntity(), nil
}

// FindAlias retrieves an Account entity from the database using the provided Alias.
func (r *AccountPostgreSQLRepository) FindAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_alias")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	builder := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"alias": alias}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		builder = builder.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	acc := &AccountPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_alias.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&acc.ID,
		&acc.Name,
		&acc.ParentAccountID,
		&acc.EntityID,
		&acc.AssetCode,
		&acc.OrganizationID,
		&acc.LedgerID,
		&acc.PortfolioID,
		&acc.SegmentID,
		&acc.Status,
		&acc.StatusDescription,
		&acc.Alias,
		&acc.Type,
		&acc.CreatedAt,
		&acc.UpdatedAt,
		&acc.DeletedAt,
		&acc.Blocked,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrAccountAliasNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			logger.Warnf("Failed to scan row: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
	}

	return acc.ToEntity(), nil
}

// FindByAlias find account from the database using Organization and Ledger id and Alias. Returns true and ErrAliasUnavailability error if the alias is already taken.
func (r *AccountPostgreSQLRepository) FindByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_by_alias")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return false, err
	}

	builder := squirrel.Select("1").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("alias LIKE ?", alias)).
		Where(squirrel.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_alias.query")

	var exists int

	err = db.QueryRowContext(ctx, query, args...).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			spanQuery.End()
			return false, nil
		}

		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return false, err
	}

	spanQuery.End()

	err = pkg.ValidateBusinessError(constant.ErrAliasUnavailability, reflect.TypeOf(mmodel.Account{}).Name(), alias)

	libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Alias is already taken", err)

	logger.Warnf("Alias is already taken: %v", alias)

	return true, err
}

// ListByIDs retrieves Accounts entities from the database using the provided IDs.
func (r *AccountPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	builder := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": ids}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		builder = builder.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return accounts, nil
}

// ListByAlias retrieves Accounts entities from the database using the provided alias.
func (r *AccountPostgreSQLRepository) ListByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias []string) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_alias")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	builder := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"portfolio_id": portfolioID}).
		Where(squirrel.Eq{"alias": alias}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_alias.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return accounts, nil
}

// Update an Account entity into Postgresql and returns the Account updated.
func (r *AccountPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &AccountPostgreSQLModel{}
	record.FromEntity(acc)

	builder := squirrel.Update(r.tableName)

	if acc.Name != "" {
		builder = builder.Set("name", record.Name)
	}

	if !acc.Status.IsEmpty() {
		builder = builder.Set("status", record.Status)
		builder = builder.Set("status_description", record.StatusDescription)
	}

	if !libCommons.IsNilOrEmpty(acc.Alias) {
		builder = builder.Set("alias", record.Alias)
	}

	if acc.Blocked != nil {
		builder = builder.Set("blocked", *acc.Blocked)
	}

	if !libCommons.IsNilOrEmpty(acc.SegmentID) {
		builder = builder.Set("segment_id", record.SegmentID)
	}

	if !libCommons.IsNilOrEmpty(acc.EntityID) {
		builder = builder.Set("entity_id", record.EntityID)
	}

	if !libCommons.IsNilOrEmpty(acc.PortfolioID) {
		builder = builder.Set("portfolio_id", record.PortfolioID)
	}

	record.UpdatedAt = time.Now()
	builder = builder.Set("updated_at", record.UpdatedAt)

	builder = builder.Where(squirrel.Eq{"organization_id": organizationID})
	builder = builder.Where(squirrel.Eq{"ledger_id": ledgerID})
	builder = builder.Where(squirrel.Eq{"id": id})
	builder = builder.Where(squirrel.Expr("deleted_at IS NULL"))

	if portfolioID != nil && *portfolioID != uuid.Nil {
		builder = builder.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	builder = builder.PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", err)

			logger.Errorf("Failed to execute update query: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account", err)

		logger.Warnf("Failed to update account: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete an Account entity from the database (soft delete) using the provided ID.
func (r *AccountPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	builder := squirrel.Update(r.tableName).
		Set("deleted_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		builder = builder.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	spanExec.End()

	return nil
}

// ListAccountsByIDs list Accounts entity from the database using the provided IDs.
func (r *AccountPostgreSQLRepository) ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	builder := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": ids}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return accounts, nil
}

// ListAccountsByAlias list Accounts entity from the database using the provided alias.
func (r *AccountPostgreSQLRepository) ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_alias")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	builder := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"alias": aliases}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_alias.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return accounts, nil
}

// Count retrieves the count of accounts from the database.
func (r *AccountPostgreSQLRepository) Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_accounts")
	defer span.End()

	var count = int64(0)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return count, err
	}

	builder := squirrel.Select("COUNT(*)").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return count, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")

	err = db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return count, err
	}

	spanQuery.End()

	return count, nil
}
