package assetrate

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

// Repository provides an interface for asset rate persistence operations.
//
// This interface defines the contract for asset rate operations, following
// the repository pattern from Domain-Driven Design. Unlike other entities,
// asset rates have no soft delete as they are versioned by creation time.
//
// Design Decisions:
//
//   - Organization and ledger scoping: All operations require both IDs
//   - Currency pair lookup: Primary query pattern for rate retrieval
//   - External ID lookup: Integration with external rate providers
//   - Cursor pagination: Efficient traversal of large result sets
//   - No delete: Rates are historical and not deleted
//
// Thread Safety:
//
// All methods are thread-safe. The underlying database driver handles connection
// pooling and concurrent access.
type Repository interface {
	Create(ctx context.Context, assetRate *AssetRate) (*AssetRate, error)
	FindByCurrencyPair(ctx context.Context, organizationID, ledgerID uuid.UUID, from, to string) (*AssetRate, error)
	FindByExternalID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*AssetRate, error)
	FindAllByAssetCodes(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, toAssetCodes []string, filter http.Pagination) ([]*AssetRate, libHTTP.CursorPagination, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, assetRate *AssetRate) (*AssetRate, error)
}

// AssetRatePostgreSQLRepository is the PostgreSQL implementation of the asset rate Repository.
//
// This struct implements the Repository interface using PostgreSQL as the underlying
// data store. It manages asset rate persistence with proper connection handling,
// distributed tracing, and structured logging.
//
// Connection Management:
//
// The repository holds a reference to a PostgresConnection which manages the actual
// database connection pool. The connection is validated during construction.
//
// Lifecycle:
//
//	// Initialization (typically in dependency injection)
//	conn := libPostgres.NewPostgresConnection(cfg)
//	repo := assetrate.NewAssetRatePostgreSQLRepository(conn)
//
//	// Usage in service layer
//	rate, err := repo.Create(ctx, newRate)
//	rate, err := repo.FindByCurrencyPair(ctx, orgID, ledgerID, "USD", "BRL")
//
//	// Connection lifecycle managed by PostgresConnection
//
// Thread Safety:
//
// AssetRatePostgreSQLRepository is safe for concurrent use. The underlying
// PostgresConnection manages a connection pool that handles concurrent access.
//
// Fields:
//   - connection: PostgreSQL connection pool wrapper
//   - tableName: Target table name ("asset_rate")
type AssetRatePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewAssetRatePostgreSQLRepository constructs a new AssetRatePostgreSQLRepository.
//
// This constructor initializes the repository with a validated database connection.
// It verifies connectivity before returning to ensure fail-fast behavior during
// application startup.
//
// Initialization Process:
//  1. Store PostgreSQL connection reference
//  2. Set target table name to "asset_rate"
//  3. Validate database connectivity
//  4. Panic if connection fails (fail-fast for misconfiguration)
//
// Parameters:
//   - pc: PostgreSQL connection wrapper (must not be nil)
//
// Returns:
//   - *AssetRatePostgreSQLRepository: Ready-to-use repository instance
//
// Panics:
//
// This function panics if the database connection cannot be established.
// This is intentional to prevent the application from starting with an
// invalid database configuration.
//
// Example:
//
//	conn := libPostgres.NewPostgresConnection(cfg)
//	repo := assetrate.NewAssetRatePostgreSQLRepository(conn)
//	// repo is now ready for use
func NewAssetRatePostgreSQLRepository(pc *libPostgres.PostgresConnection) *AssetRatePostgreSQLRepository {
	c := &AssetRatePostgreSQLRepository{
		connection: pc,
		tableName:  "asset_rate",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create persists a new asset rate entity to PostgreSQL.
//
// This method creates a new asset rate record with a generated UUID v7 identifier.
// It uses distributed tracing for observability and validates that the record
// was successfully inserted.
//
// Process:
//  1. Extract logger and tracer from context
//  2. Start tracing span for the operation
//  3. Get database connection from pool
//  4. Convert domain model to PostgreSQL model (generates new ID)
//  5. Execute INSERT query with all fields
//  6. Validate rows affected > 0
//  7. Return converted domain entity
//
// Parameters:
//   - ctx: Context for tracing, cancellation, and deadline propagation
//   - assetRate: Domain asset rate to persist (ID will be generated)
//
// Returns:
//   - *AssetRate: Created asset rate with generated ID and timestamps
//   - error: ErrEntityNotFound if insert affected 0 rows, or database errors
//
// Observability:
//
// Creates spans: "postgres.create_asset_rate", "postgres.create.exec"
func (r *AssetRatePostgreSQLRepository) Create(ctx context.Context, assetRate *AssetRate) (*AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}
	record.FromEntity(assetRate)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	result, err := db.ExecContext(ctx, `INSERT INTO asset_rate VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *`,
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.ExternalID,
		&record.From,
		&record.To,
		&record.Rate,
		&record.RateScale,
		&record.Source,
		&record.TTL,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		logger.Errorf("Failed to execute insert query: %v", err)

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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset rate. Rows affected is 0", err)

		logger.Errorf("Failed to create asset rate. Rows affected is 0: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByExternalID retrieves the most recent asset rate by external identifier.
//
// This method looks up an asset rate using an external system identifier,
// useful for integrating with external rate providers (e.g., forex APIs).
// When multiple rates exist for the same external ID, the most recent is returned.
//
// Process:
//  1. Extract logger and tracer from context
//  2. Start tracing span for the operation
//  3. Get database connection from pool
//  4. Execute query with organization, ledger, and external ID filters
//  5. Order by created_at DESC to get latest rate
//  6. Scan result into model and convert to domain entity
//
// Parameters:
//   - ctx: Context for tracing, cancellation, and deadline propagation
//   - organizationID: Organization scope for multi-tenancy
//   - ledgerID: Ledger scope within organization
//   - externalID: External system identifier
//
// Returns:
//   - *AssetRate: Found asset rate entity
//   - error: ErrEntityNotFound if no matching rate, or database errors
//
// Observability:
//
// Creates spans: "postgres.find_asset_rate_by_external_id", "postgres.find.query"
func (r *AssetRatePostgreSQLRepository) FindByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_rate_by_external_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, `SELECT * FROM asset_rate WHERE organization_id = $1 AND ledger_id = $2 AND external_id = $3 ORDER BY created_at DESC`, organizationID, ledgerID, externalID)

	spanQuery.End()

	if err := row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.ExternalID,
		&record.From,
		&record.To,
		&record.Rate,
		&record.RateScale,
		&record.Source,
		&record.TTL,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find asset rate. Row not found", err)

			logger.Errorf("Failed to find asset rate. Row not found: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		logger.Errorf("Failed to scan asset rate record: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByCurrencyPair retrieves the most recent exchange rate between two assets.
//
// This is the primary query method for asset rates, looking up the conversion
// rate from one asset to another. When multiple rates exist for the same pair,
// the most recently created rate is returned.
//
// Process:
//  1. Extract logger and tracer from context
//  2. Start tracing span for the operation
//  3. Get database connection from pool
//  4. Execute query with organization, ledger, and currency pair filters
//  5. Order by created_at DESC to get latest rate
//  6. Return nil (not error) if no rate found
//
// Parameters:
//   - ctx: Context for tracing, cancellation, and deadline propagation
//   - organizationID: Organization scope for multi-tenancy
//   - ledgerID: Ledger scope within organization
//   - from: Source asset code (e.g., "USD")
//   - to: Target asset code (e.g., "BRL")
//
// Returns:
//   - *AssetRate: Found asset rate, or nil if no rate exists for the pair
//   - error: Database errors only (not-found returns nil, nil)
//
// Observability:
//
// Creates spans: "postgres.find_asset_rate_by_currency_pair", "postgres.find.query"
func (r *AssetRatePostgreSQLRepository) FindByCurrencyPair(ctx context.Context, organizationID, ledgerID uuid.UUID, from, to string) (*AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_rate_by_currency_pair")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, `SELECT * FROM asset_rate WHERE organization_id = $1 AND ledger_id = $2 AND "from" = $3 AND "to" = $4 ORDER BY created_at DESC`, organizationID, ledgerID, from, to)

	spanQuery.End()

	if err := row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.ExternalID,
		&record.From,
		&record.To,
		&record.Rate,
		&record.RateScale,
		&record.Source,
		&record.TTL,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Errorf("Asset rate not found: %v", err)

			return nil, nil
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		logger.Errorf("Failed to scan asset rate record: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindAllByAssetCodes retrieves paginated asset rates for a source asset to multiple targets.
//
// This method supports cursor-based pagination for efficiently traversing large
// result sets. It finds all rates from a single source asset to multiple target
// assets, with optional date range filtering.
//
// Process:
//  1. Extract logger and tracer from context
//  2. Decode cursor if provided (for pagination continuation)
//  3. Build query with Squirrel query builder
//  4. Apply filters: organization, ledger, from asset, to assets, date range
//  5. Apply cursor pagination logic
//  6. Execute query and scan results
//  7. Calculate next/previous cursors for navigation
//
// Parameters:
//   - ctx: Context for tracing, cancellation, and deadline propagation
//   - organizationID: Organization scope for multi-tenancy
//   - ledgerID: Ledger scope within organization
//   - fromAssetCode: Source asset code to find rates from
//   - toAssetCodes: Target asset codes to filter (nil for all)
//   - filter: Pagination parameters including limit, cursor, date range
//
// Returns:
//   - []*AssetRate: Paginated list of matching asset rates
//   - CursorPagination: Cursor information for next/previous page navigation
//   - error: Database or cursor decoding errors
//
// Observability:
//
// Creates spans: "postgres.find_all_asset_rates_by_asset_codes", "postgres.find_all.query"
func (r *AssetRatePostgreSQLRepository) FindAllByAssetCodes(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, toAssetCodes []string, filter http.Pagination) ([]*AssetRate, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_asset_rates_by_asset_codes")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	assetRates := make([]*AssetRate, 0)

	decodedCursor := libHTTP.Cursor{}
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !isFirstPage {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr(`"from" = ?`, fromAssetCode)).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	if toAssetCodes != nil {
		findAll.Where(squirrel.Eq{`"to"`: toAssetCodes})
	}

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var assetRate AssetRatePostgreSQLModel
		if err := rows.Scan(
			&assetRate.ID,
			&assetRate.OrganizationID,
			&assetRate.LedgerID,
			&assetRate.ExternalID,
			&assetRate.From,
			&assetRate.To,
			&assetRate.Rate,
			&assetRate.RateScale,
			&assetRate.Source,
			&assetRate.TTL,
			&assetRate.CreatedAt,
			&assetRate.UpdatedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		assetRates = append(assetRates, assetRate.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(assetRates) > filter.Limit

	assetRates = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, assetRates, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(assetRates) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, assetRates[0].ID, assetRates[len(assetRates)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return assetRates, cur, nil
}

// Update modifies an existing asset rate in PostgreSQL.
//
// This method performs a partial update on an asset rate, only modifying fields
// that have changed. The source field is conditionally updated only if provided.
// The updated_at timestamp is always refreshed.
//
// Process:
//  1. Extract logger and tracer from context
//  2. Start tracing span for the operation
//  3. Get database connection from pool
//  4. Convert domain model to PostgreSQL model
//  5. Build dynamic UPDATE query with conditional fields
//  6. Execute query with organization, ledger, and ID filters
//  7. Validate rows affected > 0
//  8. Return updated entity
//
// Parameters:
//   - ctx: Context for tracing, cancellation, and deadline propagation
//   - organizationID: Organization scope for multi-tenancy
//   - ledgerID: Ledger scope within organization
//   - id: Asset rate unique identifier
//   - assetRate: Domain model with updated values
//
// Returns:
//   - *AssetRate: Updated asset rate entity
//   - error: ErrEntityNotFound if no matching rate, or database errors
//
// Observability:
//
// Creates spans: "postgres.update_asset_rate", "postgres.update.exec"
func (r *AssetRatePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, assetRate *AssetRate) (*AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}
	record.FromEntity(assetRate)

	var updates []string

	var args []any

	if !libCommons.IsNilOrEmpty(assetRate.Source) {
		updates = append(updates, "source = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Source)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates,
		"updated_at = $"+strconv.Itoa(len(args)+1),
		"rate = $"+strconv.Itoa(len(args)+2),
		"rate_scale = $"+strconv.Itoa(len(args)+3),
		"ttl = $"+strconv.Itoa(len(args)+4),
		"external_id = $"+strconv.Itoa(len(args)+5),
	)

	args = append(args, record.UpdatedAt, record.Rate, record.RateScale, record.TTL, record.ExternalID, organizationID, ledgerID, id)

	query := `UPDATE asset_rate SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args))

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update asset rate. Rows affected is 0", err)

		logger.Warnf("Failed to update asset rate. Rows affected is 0: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}
