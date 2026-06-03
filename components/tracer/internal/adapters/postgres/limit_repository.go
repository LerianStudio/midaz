// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v3/components/tracer/pkg/net/http"
)

// LimitRepository implements limitsvc.LimitRepository using PostgreSQL with Squirrel query builder.
// It provides CRUD operations for limits with cursor-based pagination, soft delete support,
// and OpenTelemetry distributed tracing integration.
//
// Tenant resolution is handled by the underlying pgdb.Connection (M1).
type LimitRepository struct {
	conn      pgdb.Connection
	tableName string
}

// NewLimitRepositoryWithConnection creates a new PostgreSQL limit repository with a custom pgdb.Connection.
// This is primarily used for testing with mock connections.
func NewLimitRepositoryWithConnection(conn pgdb.Connection) *LimitRepository {
	return &LimitRepository{
		conn:      conn,
		tableName: "limits",
	}
}

// limitSortFieldToColumn maps API snake_case sort fields to database column names.
var limitSortFieldToColumn = map[string]string{
	"created_at": "created_at",
	"updated_at": "updated_at",
	"max_amount": "max_amount",
	"name":       "name",
}

// mapLimitSortFieldToColumn converts a sort field to its database column name.
// Returns the column name if valid, otherwise returns empty string.
func mapLimitSortFieldToColumn(sortField string) string {
	if col, ok := limitSortFieldToColumn[sortField]; ok {
		return col
	}

	return ""
}

// CreateWithTx inserts a new limit using the provided database handle.
// Callers typically pass a pgdb.Tx so the insert participates in an external
// transaction (for example, alongside an audit event insert). The db handle
// MUST be non-nil; passing nil returns pgdb.ErrNilConnection so the atomicity
// guarantee cannot be silently downgraded.
func (r *LimitRepository) CreateWithTx(ctx context.Context, db pgdb.DB, lmt *model.Limit) error {
	if db == nil {
		return pgdb.ErrNilConnection
	}

	return r.createInternal(ctx, db, lmt)
}

// createInternal executes the INSERT statement for a limit on the provided db
// handle. Shared by write paths so the SQL, logging, and error
// handling live in one place. When db is nil, the connection is resolved via
// r.conn.GetDB so the span covers GetDB failures.
func (r *LimitRepository) createInternal(ctx context.Context, db pgdb.DB, lmt *model.Limit) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.limit.create")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if db == nil {
		var err error

		db, err = r.conn.GetDB(ctx)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to get database connection", err)
			return fmt.Errorf("failed to get database connection: %w", err)
		}
	}

	// Convert entity to database model using ToEntity/FromEntity pattern
	var dbModel LimitPostgreSQLModel
	if err := dbModel.FromEntity(lmt); err != nil {
		libOtel.HandleSpanError(span, "Failed to convert entity to database model", err)
		return fmt.Errorf("failed to convert entity to database model: %w", err)
	}

	query := sq.Insert(r.tableName).
		Columns("id", "name", "description", "limit_type", "max_amount", "currency", "scopes", "status", "reset_at", "active_time_start", "active_time_end", "custom_start_date", "custom_end_date", "created_at", "updated_at").
		Values(dbModel.ID, dbModel.Name, dbModel.Description, dbModel.LimitType, dbModel.MaxAmount, dbModel.Currency, dbModel.Scopes, dbModel.Status, dbModel.ResetAt, dbModel.ActiveTimeStart, dbModel.ActiveTimeEnd, dbModel.CustomStartDate, dbModel.CustomEndDate, dbModel.CreatedAt, dbModel.UpdatedAt).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.limit.create"),
		libLog.String("limit.id", lmt.ID.String()),
		libLog.String("limit.name", lmt.Name),
	).Log(ctx, libLog.LevelInfo, "Creating limit")

	_, err = db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		if IsUniqueViolationOf(err, "idx_limits_name_active") {
			libOtel.HandleSpanBusinessErrorEvent(span, "Limit name already exists", constant.ErrLimitNameAlreadyExists)
			return constant.ErrLimitNameAlreadyExists
		}

		libOtel.HandleSpanError(span, "Failed to insert limit", err)

		return fmt.Errorf("failed to insert limit: %w", err)
	}

	return nil
}

// GetByID retrieves a limit by its ID.
func (r *LimitRepository) GetByID(ctx context.Context, limitID uuid.UUID) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.limit.get_by_id")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := sq.Select("id", "name", "description", "limit_type", "max_amount", "currency", "scopes", "status", "reset_at", "active_time_start", "active_time_end", "custom_start_date", "custom_end_date", "created_at", "updated_at", "deleted_at").
		From(r.tableName).
		Where(sq.Eq{"id": limitID}).
		Where(sq.Eq{"deleted_at": nil}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.limit.get_by_id"),
		libLog.String("limit.id", limitID.String()),
	).Log(ctx, libLog.LevelInfo, "Getting limit by ID")

	lmt, err := r.scanLimit(ctx, db.QueryRowContext(ctx, sqlStr, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Limit not found", constant.ErrLimitNotFound)
			return nil, constant.ErrLimitNotFound
		}

		libOtel.HandleSpanError(span, "Failed to get limit", err)

		return nil, fmt.Errorf("failed to get limit: %w", err)
	}

	return lmt, nil
}

// List retrieves limits with optional filters and cursor-based pagination.
func (r *LimitRepository) List(ctx context.Context, filters *model.ListLimitsFilter) (*model.ListLimitsResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.limit.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	filters = r.normalizeListFilters(filters)

	sortColumn, sortOrder, err := r.validateAndNormalizeSort(filters)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)
		return nil, err
	}

	// Keep original sortBy for cursor encoding
	sortBy := filters.SortBy
	if sortBy == "" {
		sortBy = model.DefaultLimitSortField
	}

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := sq.Select("id", "name", "description", "limit_type", "max_amount", "currency", "scopes", "status", "reset_at", "active_time_start", "active_time_end", "custom_start_date", "custom_end_date", "created_at", "updated_at", "deleted_at").
		From(r.tableName).
		Where(sq.Eq{"deleted_at": nil}).
		PlaceholderFormat(sq.Dollar)

	query = r.applyListFilters(query, filters)

	// Apply cursor filter for keyset pagination (uses snake_case sortColumn for queries)
	query, sortColumn, sortOrder, err = r.applyCursorFilter(query, filters.Cursor, sortColumn, sortOrder, span)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)
		return nil, err
	}

	// Apply ordering (uses snake_case sortColumn)
	query = r.applyOrderBy(query, sortColumn, sortOrder)

	// Fetch Limit+1 to determine if more pages exist
	fetchLimit := filters.Limit + 1
	// Defense-in-depth: ensure fetchLimit is positive before uint64 conversion
	// to prevent integer overflow (gosec G115). Validation at upstream layers
	// already ensures Limit >= 0, but we add local protection.
	if fetchLimit <= 0 {
		// This should never happen due to upstream validation, but protect against
		// potential bypass or refactoring. Use default limit + 1 as safe fallback.
		fetchLimit = constant.DefaultPaginationLimit + 1
	}

	query = query.Limit(uint64(fetchLimit)) // #nosec G115 - fetchLimit validated positive above

	sqlStr, args, err := query.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.limit.list"),
		libLog.Int("filter.limit", filters.Limit),
		libLog.Bool("filter.has_cursor", filters.Cursor != ""),
	).Log(ctx, libLog.LevelInfo, "Listing limits")

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to list limits", err)
		return nil, fmt.Errorf("failed to list limits: %w", err)
	}
	defer rows.Close()

	var limits []model.Limit

	for rows.Next() {
		lmt, err := r.scanLimitFromRows(ctx, rows)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to scan limit", err)
			return nil, fmt.Errorf("failed to scan limit: %w", err)
		}

		limits = append(limits, *lmt)
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Error iterating limits", err)
		return nil, fmt.Errorf("error iterating limits: %w", err)
	}

	// Determine if there are more results
	hasMore := len(limits) > filters.Limit

	if hasMore {
		limits = limits[:filters.Limit]
	}

	// Generate next cursor from the last item
	var nextCursor string

	if hasMore && len(limits) > 0 {
		lastLimit := limits[len(limits)-1]

		// Use sortBy in cursor for encoding
		nextCursor, err = r.buildNextCursor(&lastLimit, sortBy, sortOrder)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to encode cursor", err)
			return nil, fmt.Errorf("failed to encode cursor: %w", err)
		}
	}

	result := &model.ListLimitsResult{
		Limits:     limits,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}

	logger.With(
		libLog.String("operation", "repository.limit.list"),
		libLog.Int("result.count", len(limits)),
		libLog.Bool("result.has_more", hasMore),
	).Log(ctx, libLog.LevelInfo, "Listed limits")

	return result, nil
}

// UpdateWithTx modifies an existing limit using the provided database handle.
// Callers typically pass a pgdb.Tx so the update participates in an external
// transaction (for example, alongside an audit event insert). The db handle
// MUST be non-nil; passing nil returns pgdb.ErrNilConnection so the atomicity
// guarantee cannot be silently downgraded.
//
// Invariant: every tracked column (including updated_at) is written from the
// in-memory lmt value. No DB-side triggers are allowed to mutate tracked
// columns (e.g. BEFORE UPDATE SET updated_at = now()) because audit consumers
// capture afterState in memory from lmt post-mutation and rely on it matching
// the persisted row. If a trigger is ever introduced, audit consumers must
// switch to re-reading the row post-UPDATE instead.
func (r *LimitRepository) UpdateWithTx(ctx context.Context, db pgdb.DB, lmt *model.Limit) error {
	if db == nil {
		return pgdb.ErrNilConnection
	}

	return r.updateInternal(ctx, db, lmt)
}

// updateInternal executes the UPDATE statement for a limit on the provided
// db handle. Shared by Update and UpdateWithTx so the SQL, logging, and error
// handling live in one place. When db is nil, the connection is resolved via
// r.conn.GetDB so the span covers GetDB failures.
func (r *LimitRepository) updateInternal(ctx context.Context, db pgdb.DB, lmt *model.Limit) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.limit.update")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if db == nil {
		var err error

		db, err = r.conn.GetDB(ctx)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to get database connection", err)
			return fmt.Errorf("failed to get database connection: %w", err)
		}
	}

	// Convert entity to database model using ToEntity/FromEntity pattern
	var dbModel LimitPostgreSQLModel
	if err := dbModel.FromEntity(lmt); err != nil {
		libOtel.HandleSpanError(span, "Failed to convert entity to database model", err)
		return fmt.Errorf("failed to convert entity to database model: %w", err)
	}

	query := sq.Update(r.tableName).
		Set("name", dbModel.Name).
		Set("description", dbModel.Description).
		Set("max_amount", dbModel.MaxAmount).
		Set("scopes", dbModel.Scopes).
		Set("status", dbModel.Status).
		Set("reset_at", dbModel.ResetAt).
		Set("active_time_start", dbModel.ActiveTimeStart).
		Set("active_time_end", dbModel.ActiveTimeEnd).
		Set("custom_start_date", dbModel.CustomStartDate).
		Set("custom_end_date", dbModel.CustomEndDate).
		Set("updated_at", dbModel.UpdatedAt).
		Where(sq.Eq{"id": dbModel.ID}).
		Where(sq.Eq{"deleted_at": nil}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.limit.update"),
		libLog.String("limit.id", lmt.ID.String()),
	).Log(ctx, libLog.LevelInfo, "Updating limit")

	result, err := db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		if IsUniqueViolationOf(err, "idx_limits_name_active") {
			libOtel.HandleSpanBusinessErrorEvent(span, "Limit name already exists", constant.ErrLimitNameAlreadyExists)
			return constant.ErrLimitNameAlreadyExists
		}

		libOtel.HandleSpanError(span, "Failed to update limit", err)

		return fmt.Errorf("failed to update limit: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get rows affected", err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		libOtel.HandleSpanBusinessErrorEvent(span, "Limit not found", constant.ErrLimitNotFound)
		return constant.ErrLimitNotFound
	}

	return nil
}

// UpdateStatus updates only the status of a limit.
// If transitioning to DELETED status, also sets deleted_at for soft-delete consistency.
// Resolves the DB connection from r.conn; for transactional updates use
// UpdateStatusWithTx and pass an explicit pgdb.DB.
func (r *LimitRepository) UpdateStatus(ctx context.Context, limitID uuid.UUID, status model.LimitStatus, updatedAt time.Time) error {
	return r.updateStatusInternal(ctx, nil, limitID, status, updatedAt)
}

// UpdateStatusWithTx updates the status of a limit using the provided database
// handle. Callers typically pass a pgdb.Tx so the update participates in an
// external transaction (for example, alongside an audit event insert). The db
// handle MUST be non-nil; callers that want a non-transactional update must
// invoke UpdateStatus instead. Passing nil here returns pgdb.ErrNilConnection
// so the atomicity guarantee cannot be silently downgraded.
func (r *LimitRepository) UpdateStatusWithTx(ctx context.Context, db pgdb.DB, limitID uuid.UUID, status model.LimitStatus, updatedAt time.Time) error {
	if db == nil {
		return pgdb.ErrNilConnection
	}

	return r.updateStatusInternal(ctx, db, limitID, status, updatedAt)
}

// updateStatusInternal executes the status UPDATE statement for a limit on the
// provided db handle. Shared by UpdateStatus and UpdateStatusWithTx. When db is
// nil, the connection is resolved via r.conn.GetDB so the span covers GetDB
// failures.
func (r *LimitRepository) updateStatusInternal(ctx context.Context, db pgdb.DB, limitID uuid.UUID, status model.LimitStatus, updatedAt time.Time) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.limit.update_status")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if db == nil {
		var err error

		db, err = r.conn.GetDB(ctx)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to get database connection", err)
			return fmt.Errorf("failed to get database connection: %w", err)
		}
	}

	query := sq.Update(r.tableName).
		Set("status", status).
		Set("updated_at", updatedAt)

	// Handle deleted_at for DELETED status transitions
	if status == model.LimitStatusDeleted {
		query = query.Set("deleted_at", updatedAt)
	}

	query = query.
		Where(sq.Eq{"id": limitID}).
		Where(sq.Eq{"deleted_at": nil}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.limit.update_status"),
		libLog.String("limit.id", limitID.String()),
		libLog.String("status", string(status)),
	).Log(ctx, libLog.LevelInfo, "Updating limit status")

	result, err := db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to update limit status", err)
		return fmt.Errorf("failed to update limit status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get rows affected", err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		libOtel.HandleSpanBusinessErrorEvent(span, "Limit not found", constant.ErrLimitNotFound)
		return constant.ErrLimitNotFound
	}

	return nil
}

// applyCursorFilter adds keyset pagination WHERE clause to the query.
// Supports custom sort columns with id as tiebreaker.
// Returns the updated query, sort column, and sort order from the cursor (for consistency).
func (r *LimitRepository) applyCursorFilter(query sq.SelectBuilder, cursorStr string, requestedSortBy string, requestedOrderDir string, span trace.Span) (sq.SelectBuilder, string, string, error) {
	if cursorStr == "" {
		return query, requestedSortBy, requestedOrderDir, nil
	}

	cursor, err := pkgHTTP.DecodeCursor(cursorStr)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)
		return query, requestedSortBy, requestedOrderDir, fmt.Errorf("%w: %w", constant.ErrInvalidCursor, err)
	}

	// Use sort field from cursor (in snake_case) and validate
	sortField := cursor.SortBy
	if sortField == "" {
		sortField = model.DefaultLimitSortField
	} else if !model.IsValidLimitSortField(sortField) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid sort column in cursor", constant.ErrInvalidSortColumn)
		return query, requestedSortBy, requestedOrderDir, constant.ErrInvalidSortColumn
	}

	// Map API field to database column (snake_case to snake_case)
	sortColumn := mapLimitSortFieldToColumn(sortField)
	if sortColumn == "" {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid sort column in cursor", constant.ErrInvalidSortColumn)
		return query, requestedSortBy, requestedOrderDir, constant.ErrInvalidSortColumn
	}

	orderDir := cursor.SortOrder
	if orderDir != "ASC" && orderDir != "DESC" {
		orderDir = "DESC"
	}

	// Validate cursor sort parameters match request parameters
	// This prevents clients from changing sort mid-pagination which could cause inconsistent results
	if sortColumn != requestedSortBy {
		libOtel.HandleSpanBusinessErrorEvent(span, "Cursor sort mismatch", constant.ErrInvalidCursor)
		return query, requestedSortBy, requestedOrderDir, fmt.Errorf("%w: cursor sortBy does not match request", constant.ErrInvalidCursor)
	}

	if orderDir != strings.ToUpper(requestedOrderDir) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Cursor sort order mismatch", constant.ErrInvalidCursor)
		return query, requestedSortBy, requestedOrderDir, fmt.Errorf("%w: cursor sortOrder does not match request", constant.ErrInvalidCursor)
	}

	// Validate cursor sort value type matches expected column type
	// Use sortField (snake_case) as validateCursorSortValue expects API field names
	if err := validateCursorSortValue(sortField, cursor.SortValue); err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid cursor sort value type", constant.ErrInvalidCursor)
		return query, requestedSortBy, requestedOrderDir, fmt.Errorf("%w: %w", constant.ErrInvalidCursor, err)
	}

	// Build WHERE clause based on sort column
	query = r.buildCursorCondition(query, &cursor, sortColumn, orderDir)

	return query, sortColumn, orderDir, nil
}

// buildCursorCondition builds WHERE clause for keyset pagination.
// Uses sort value + ID as tiebreaker for consistent pagination.
func (r *LimitRepository) buildCursorCondition(query sq.SelectBuilder, cursor *pkgHTTP.Cursor, sortBy, sortOrder string) sq.SelectBuilder {
	lt := sq.Lt{}
	gt := sq.Gt{}
	eq := sq.Eq{}

	lt[sortBy] = cursor.SortValue
	gt[sortBy] = cursor.SortValue
	eq[sortBy] = cursor.SortValue

	if sortOrder == "DESC" {
		return query.Where(
			sq.Or{
				lt, // sort_value < cursor
				sq.And{
					eq,                     // sort_value = cursor
					sq.Lt{"id": cursor.ID}, // AND id < cursor
				},
			},
		)
	}

	return query.Where(
		sq.Or{
			gt, // sort_value > cursor
			sq.And{
				eq,                     // sort_value = cursor
				sq.Gt{"id": cursor.ID}, // AND id > cursor
			},
		},
	)
}

// applyOrderBy applies ORDER BY clause for keyset pagination.
// Always adds id as secondary sort for stable pagination.
// sortBy is validated against validSortColumns whitelist before calling this method.
// sortOrder is constrained to "ASC" or "DESC" before calling this method.
func (r *LimitRepository) applyOrderBy(query sq.SelectBuilder, sortBy, sortOrder string) sq.SelectBuilder {
	// Use string concatenation instead of fmt.Sprintf
	// sortBy and sortOrder are pre-validated by whitelist and constraint checks
	return query.OrderBy(sortBy + " " + sortOrder + ", id " + sortOrder)
}

// buildNextCursor creates a base64-encoded cursor from the last limit in the result set.
// Validates sortBy against allowed fields and normalizes sortOrder to uppercase.
func (r *LimitRepository) buildNextCursor(lmt *model.Limit, sortBy, sortOrder string) (string, error) {
	// Validate sortBy against whitelist
	if !model.IsValidLimitSortField(sortBy) {
		return "", constant.ErrInvalidSortColumn
	}

	// Normalize sortOrder to uppercase
	normalizedSortOrder := strings.ToUpper(sortOrder)
	if normalizedSortOrder != "ASC" && normalizedSortOrder != "DESC" {
		normalizedSortOrder = "DESC"
	}

	sortValue := getSortValueFromLimit(lmt, sortBy)

	cursor := pkgHTTP.Cursor{
		ID:         lmt.ID.String(),
		SortValue:  sortValue,
		SortBy:     sortBy,
		SortOrder:  normalizedSortOrder,
		PointsNext: true,
	}

	return pkgHTTP.EncodeCursor(cursor)
}

// normalizeListFilters applies default values to the filter.
// Handles nil filter, zero/negative limit, and limit bounds.
func (r *LimitRepository) normalizeListFilters(filters *model.ListLimitsFilter) *model.ListLimitsFilter {
	if filters == nil {
		return &model.ListLimitsFilter{Limit: constant.DefaultPaginationLimit}
	}

	if filters.Limit <= 0 {
		filters.Limit = constant.DefaultPaginationLimit
	} else if filters.Limit > constant.MaxPaginationLimit {
		filters.Limit = constant.MaxPaginationLimit
	}

	return filters
}

// applyListFilters adds status, limit_type, name, and scope WHERE clauses to the query.
func (r *LimitRepository) applyListFilters(query sq.SelectBuilder, filters *model.ListLimitsFilter) sq.SelectBuilder {
	if filters.Name != nil && *filters.Name != "" {
		// Case-insensitive partial match using ILIKE with % wildcards
		// Escape LIKE special characters to prevent unintended pattern matching
		escapedName := escapeLikePattern(*filters.Name)
		query = query.Where(sq.ILike{"name": "%" + escapedName + "%"})
	}

	if filters.Status != nil {
		query = query.Where(sq.Eq{"status": string(*filters.Status)})
	}

	if filters.LimitType != nil {
		query = query.Where(sq.Eq{"limit_type": string(*filters.LimitType)})
	}

	if filters.Currency != nil {
		normalizedCurrency := strings.ToUpper(*filters.Currency)
		query = query.Where(sq.Eq{"currency": normalizedCurrency})
	}

	// Apply scope filter using shared buildScopeFilter() JSONB logic
	if filters.ScopeFilter != nil && !filters.ScopeFilter.IsEmpty() {
		scopeFilter, filterArgs := buildScopeFilter([]model.Scope{*filters.ScopeFilter})
		query = query.Where(scopeFilter, filterArgs...)
	}

	return query
}

// validateAndNormalizeSort validates and normalizes sort parameters.
// Returns the validated sortBy (as snake_case column name), sortOrder values, and any validation error.
func (r *LimitRepository) validateAndNormalizeSort(filters *model.ListLimitsFilter) (string, string, error) {
	sortBy := filters.SortBy
	if sortBy == "" {
		sortBy = model.DefaultLimitSortField
	}

	if !model.IsValidLimitSortField(sortBy) {
		return "", "", constant.ErrInvalidSortColumn
	}

	// Map API field to database column name
	sortColumn := mapLimitSortFieldToColumn(sortBy)
	if sortColumn == "" {
		return "", "", constant.ErrInvalidSortColumn
	}

	sortOrder := strings.ToUpper(filters.SortOrder)
	if sortOrder == "" {
		sortOrder = "DESC"
	}

	// Defense-in-depth: default invalid sortOrder to "DESC" rather than returning an error,
	// since sortOrder is already validated at the API layer and this provides safe fallback.
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "DESC"
	}

	return sortColumn, sortOrder, nil
}

// validateCursorSortValue validates that the cursor sort value has the correct type
// for the given sort field. This prevents database type coercion errors and
// unexpected query results. sortBy is expected in snake_case (API format).
func validateCursorSortValue(sortBy, sortValue string) error {
	switch sortBy {
	case "created_at", "updated_at":
		// Timestamp columns expect RFC3339Nano format
		if _, err := time.Parse(time.RFC3339Nano, sortValue); err != nil {
			return fmt.Errorf("invalid timestamp format for %s", sortBy)
		}
	case "max_amount":
		// Decimal column expects numeric string (integer or decimal format)
		if _, err := decimal.NewFromString(sortValue); err != nil {
			return fmt.Errorf("invalid decimal format for %s", sortBy)
		}
	case "name":
		// String values are acceptable as-is
	default:
		return fmt.Errorf("unsupported sort column: %s", sortBy)
	}

	return nil
}

// getSortValueFromLimit extracts the value of the sort field from a limit.
// sortBy is expected in snake_case (API format).
func getSortValueFromLimit(lmt *model.Limit, sortBy string) string {
	switch sortBy {
	case "name":
		return lmt.Name
	case "max_amount":
		return lmt.MaxAmount.String()
	case "updated_at":
		return lmt.UpdatedAt.Format(time.RFC3339Nano)
	case "created_at":
		return lmt.CreatedAt.Format(time.RFC3339Nano)
	default:
		return lmt.CreatedAt.Format(time.RFC3339Nano)
	}
}

// scanLimit scans a single row into a Limit model using the ToEntity/FromEntity pattern.
func (r *LimitRepository) scanLimit(ctx context.Context, row *sql.Row) (*model.Limit, error) {
	var (
		dbModel    LimitPostgreSQLModel
		scopesJSON []byte
	)

	// Check for context cancellation before processing
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	err := row.Scan(
		&dbModel.ID,
		&dbModel.Name,
		&dbModel.Description,
		&dbModel.LimitType,
		&dbModel.MaxAmount,
		&dbModel.Currency,
		&scopesJSON,
		&dbModel.Status,
		&dbModel.ResetAt,
		&dbModel.ActiveTimeStart,
		&dbModel.ActiveTimeEnd,
		&dbModel.CustomStartDate,
		&dbModel.CustomEndDate,
		&dbModel.CreatedAt,
		&dbModel.UpdatedAt,
		&dbModel.DeletedAt,
	)
	if err != nil {
		return nil, err
	}

	// Convert scopesJSON to string for the model
	dbModel.Scopes = string(scopesJSON)

	// Convert database model to domain entity
	lmt, err := dbModel.ToEntity()
	if err != nil {
		return nil, fmt.Errorf("failed to convert to entity: %w", err)
	}

	// Validate scopes after deserialization
	// This ensures data integrity even if database contains invalid data
	if err := r.validateScopes(lmt.Scopes); err != nil {
		return nil, err
	}

	return lmt, nil
}

// scanLimitFromRows scans a row from Rows into a Limit model using the ToEntity/FromEntity pattern.
func (r *LimitRepository) scanLimitFromRows(ctx context.Context, rows *sql.Rows) (*model.Limit, error) {
	var (
		dbModel    LimitPostgreSQLModel
		scopesJSON []byte
	)

	// Check for context cancellation before processing
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	err := rows.Scan(
		&dbModel.ID,
		&dbModel.Name,
		&dbModel.Description,
		&dbModel.LimitType,
		&dbModel.MaxAmount,
		&dbModel.Currency,
		&scopesJSON,
		&dbModel.Status,
		&dbModel.ResetAt,
		&dbModel.ActiveTimeStart,
		&dbModel.ActiveTimeEnd,
		&dbModel.CustomStartDate,
		&dbModel.CustomEndDate,
		&dbModel.CreatedAt,
		&dbModel.UpdatedAt,
		&dbModel.DeletedAt,
	)
	if err != nil {
		return nil, err
	}

	// Convert scopesJSON to string for the model
	dbModel.Scopes = string(scopesJSON)

	// Convert database model to domain entity
	lmt, err := dbModel.ToEntity()
	if err != nil {
		return nil, fmt.Errorf("failed to convert to entity: %w", err)
	}

	// Validate scopes after deserialization
	// This ensures data integrity even if database contains invalid data
	if err := r.validateScopes(lmt.Scopes); err != nil {
		return nil, err
	}

	return lmt, nil
}

// validateScopes validates scopes after deserialization from the database.
// This ensures data integrity even if database contains invalid data.
// Validates all enum fields in each scope.
func (r *LimitRepository) validateScopes(scopes []model.Scope) error {
	for i, scope := range scopes {
		// Validate TransactionType enum
		if scope.TransactionType != nil && !scope.TransactionType.IsValid() {
			return fmt.Errorf("scope at index %d: invalid transactionType", i)
		}
		// Note: Scope currently only has TransactionType enum field
		// Add additional enum validations here as new enum fields are added to model.Scope
	}

	return nil
}
