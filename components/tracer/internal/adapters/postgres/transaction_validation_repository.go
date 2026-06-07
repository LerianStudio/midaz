// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/components/tracer/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Compile-time interface implementation checks.
var (
	_ query.TransactionValidationRepository   = (*TransactionValidationRepository)(nil)
	_ command.TransactionValidationRepository = (*TransactionValidationRepository)(nil)
)

// sortFieldToColumn maps snake_case API field names to snake_case database column names.
// Add new entries here when supporting additional sort fields.
var sortFieldToColumn = map[string]string{
	"created_at":         "created_at",
	"processing_time_ms": "processing_time_ms",
}

// validTransactionValidationDBColumns defines the whitelist of valid database column names for sorting.
// Used by buildNextCursor to validate snake_case column names.
var validTransactionValidationDBColumns = map[string]bool{
	"created_at":         true,
	"processing_time_ms": true,
}

// transactionValidationColumns returns the complete column list for SELECT queries.
// Returns a new slice each call to prevent accidental mutations.
// Shared across GetByID, FindByRequestID, and List methods to ensure consistency.
func transactionValidationColumns() []string {
	return []string{
		"id",
		"request_id",
		"transaction_type",
		"sub_type",
		"amount",
		"currency",
		"transaction_timestamp",
		"account",
		"segment",
		"portfolio",
		"merchant",
		"metadata",
		"decision",
		"reason",
		"matched_rule_ids",
		"evaluated_rule_ids",
		"limit_usage_details",
		"processing_time_ms",
		"created_at",
	}
}

// TransactionValidationRepository implements TransactionValidationRepository using PostgreSQL with Squirrel query builder.
// Handles JSONB fields (account, segment, portfolio, merchant, metadata, limit_usage_details) and
// UUID[] arrays (matched_rule_ids, evaluated_rule_ids) for transaction validation persistence.
// NOTE: Only INSERT operations are allowed - transaction validation trail is immutable per SOX/GLBA requirements.
// Tenant resolution is handled by the underlying pgdb.Connection (M1).
type TransactionValidationRepository struct {
	conn      pgdb.Connection
	tableName string
}

// NewTransactionValidationRepositoryWithConnection creates a new PostgreSQL transaction validation repository with a custom pgdb.Connection.
// This is primarily used for testing with mock connections.
func NewTransactionValidationRepositoryWithConnection(conn pgdb.Connection) *TransactionValidationRepository {
	return &TransactionValidationRepository{
		conn:      conn,
		tableName: "transaction_validations",
	}
}

// Insert creates a new transaction validation record (insert-only, no updates allowed).
// This maintains the immutability requirement for compliance (SOX/GLBA).
// Uses the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func (r *TransactionValidationRepository) Insert(ctx context.Context, validation *model.TransactionValidation) error {
	if validation == nil {
		return errors.New("validation cannot be nil")
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.transaction_validation.insert")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	return r.insertInternal(ctx, db, validation, logger, span, "repository.transaction_validation.insert")
}

// InsertWithTx creates a new transaction validation record using the provided database connection.
// This allows callers to pass either a regular DB connection or a transaction (*sql.Tx),
// enabling atomic operations with other database changes.
// This maintains the immutability requirement for compliance (SOX/GLBA).
// Uses the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func (r *TransactionValidationRepository) InsertWithTx(ctx context.Context, db pgdb.DB, validation *model.TransactionValidation) error {
	if validation == nil {
		return errors.New("validation cannot be nil")
	}

	if db == nil {
		// Span not annotated here: span starts after this check to avoid
		// OpenTelemetry overhead for invalid calls (nil db is a programming error).
		return pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.transaction_validation.insert_with_tx")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	return r.insertInternal(ctx, db, validation, logger, span, "repository.transaction_validation.insert_with_tx")
}

// insertInternal contains the shared INSERT logic for both Insert and InsertWithTx.
// It performs validation, entity conversion, query building, and execution.
// Uses the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func (r *TransactionValidationRepository) insertInternal(
	ctx context.Context,
	db pgdb.DB,
	validation *model.TransactionValidation,
	logger libLog.Logger,
	span trace.Span,
	operationName string,
) error {
	if validation == nil {
		err := errors.New("validation cannot be nil")
		libOtel.HandleSpanError(span, "Nil validation input", err)

		return err
	}

	// Convert domain entity to database model using FromEntity pattern
	var dbModel TransactionValidationPostgreSQLModel
	if err := dbModel.FromEntity(validation); err != nil {
		libOtel.HandleSpanError(span, "Failed to convert entity to database model", err)
		return fmt.Errorf("failed to convert entity to database model: %w", err)
	}

	// Convert UUID array strings to StringArray for PostgreSQL UUID[] type
	matchedRuleIDs := uuidSliceToStringArray(validation.MatchedRuleIDs)
	evaluatedRuleIDs := uuidSliceToStringArray(validation.EvaluatedRuleIDs)

	qb := sq.Insert(r.tableName).
		Columns(
			"id",
			"request_id",
			"transaction_type",
			"sub_type",
			"amount",
			"currency",
			"transaction_timestamp",
			"account",
			"segment",
			"portfolio",
			"merchant",
			"metadata",
			"decision",
			"reason",
			"matched_rule_ids",
			"evaluated_rule_ids",
			"limit_usage_details",
			"processing_time_ms",
			"created_at",
		).
		Values(
			dbModel.ID,
			dbModel.RequestID,
			dbModel.TransactionType,
			dbModel.SubType,
			dbModel.Amount,
			dbModel.Currency,
			dbModel.TransactionTimestamp,
			dbModel.Account,
			dbModel.Segment,
			dbModel.Portfolio,
			dbModel.Merchant,
			dbModel.Metadata,
			dbModel.Decision,
			dbModel.Reason,
			matchedRuleIDs,
			evaluatedRuleIDs,
			dbModel.LimitUsageDetails,
			dbModel.ProcessingTimeMs,
			dbModel.CreatedAt,
		).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)

		return fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", operationName),
		libLog.String("validation.id", validation.ID.String()),
		libLog.String("validation.decision", string(validation.Decision)),
	).Log(ctx, libLog.LevelInfo, "Inserting transaction validation record")

	_, err = db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		if IsUniqueViolation(err) {
			span.AddEvent("duplicate_request_id_detected")

			return fmt.Errorf("%w: request_id %s", command.ErrDuplicateValidation, validation.RequestID)
		}

		libOtel.HandleSpanError(span, "Failed to insert transaction validation", err)

		return fmt.Errorf("failed to insert transaction validation: %w", err)
	}

	return nil
}

// GetByID retrieves a specific transaction validation record by its unique identifier.
// Returns constant.ErrTransactionValidationNotFound if the record does not exist.
func (r *TransactionValidationRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.TransactionValidation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.transaction_validation.get_by_id")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	qb := sq.Select(transactionValidationColumns()...).
		From(r.tableName).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.transaction_validation.get_by_id"),
		libLog.String("validation.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Getting transaction validation by ID")

	validation, err := r.scanValidation(ctx, db.QueryRowContext(ctx, sqlStr, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Transaction validation not found", constant.ErrTransactionValidationNotFound)

			return nil, constant.ErrTransactionValidationNotFound
		}

		libOtel.HandleSpanError(span, "Failed to get transaction validation", err)

		return nil, fmt.Errorf("failed to get transaction validation: %w", err)
	}

	return validation, nil
}

// FindByRequestID retrieves a transaction validation record by its request ID.
// Used for idempotency checks to detect duplicate validation requests.
// Returns (nil, nil) if no record exists with the given request ID (not an error).
// Returns (validation, nil) if found.
// Returns (nil, error) for database/infrastructure errors only.
func (r *TransactionValidationRepository) FindByRequestID(ctx context.Context, requestID uuid.UUID) (*model.TransactionValidation, error) {
	if requestID == uuid.Nil {
		return nil, nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.transaction_validation.find_by_request_id")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	qb := sq.Select(transactionValidationColumns()...).
		From(r.tableName).
		Where(sq.Eq{"request_id": requestID}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.transaction_validation.find_by_request_id"),
		libLog.String("request.id", requestID.String()),
	).Log(ctx, libLog.LevelDebug, "Finding transaction validation by request ID")

	validation, err := r.scanValidation(ctx, db.QueryRowContext(ctx, sqlStr, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Not found is NOT an error for FindByRequestID - return (nil, nil)
			span.AddEvent("request_id_not_found")

			logger.With(
				libLog.String("operation", "repository.transaction_validation.find_by_request_id"),
				libLog.String("request.id", requestID.String()),
			).Log(ctx, libLog.LevelDebug, "Transaction validation not found by request ID")

			return nil, nil
		}

		libOtel.HandleSpanError(span, "Failed to find transaction validation by request ID", err)

		return nil, fmt.Errorf("failed to find transaction validation by request ID: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.transaction_validation.find_by_request_id"),
		libLog.String("request.id", requestID.String()),
		libLog.String("validation.id", validation.ID.String()),
	).Log(ctx, libLog.LevelDebug, "Found transaction validation by request ID")

	return validation, nil
}

// List retrieves transaction validation records matching the provided filters using cursor-based pagination.
// If filters is nil, defaults are applied (last 90 days, limit 100).
// Results are ordered by the specified sortBy field (default: created_at DESC).
func (r *TransactionValidationRepository) List(ctx context.Context, filters *model.TransactionValidationFilters) (*model.ListTransactionValidationsResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.transaction_validation.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Apply defaults if filters is nil
	if filters == nil {
		filters = &model.TransactionValidationFilters{}
	}

	filters.SetDefaults()

	// Validate filters
	if err := filters.Validate(); err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid transaction validation filters", err)
		return nil, fmt.Errorf("%w: %w", constant.ErrInvalidTransactionValidationFilters, err)
	}

	// Validate and normalize sort parameters
	sortBy, sortOrder, err := r.validateAndNormalizeSort(filters)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid sort column", err)
		return nil, err
	}

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	qb := sq.Select(transactionValidationColumns()...).
		From(r.tableName).
		PlaceholderFormat(sq.Dollar)

	// Apply business filters
	qb = r.applyFilters(qb, filters)

	// Apply cursor filter for keyset pagination
	qb, sortBy, sortOrder, err = r.applyCursorFilter(qb, filters.Cursor, sortBy, sortOrder, span)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)
		return nil, err
	}

	// Apply ordering
	qb = r.applyOrderBy(qb, sortBy, sortOrder)

	// Fetch Limit+1 to determine if more pages exist
	// Defense-in-depth: ensure fetchLimit is positive before uint64 conversion
	// to prevent integer overflow (gosec G115). Validation at model layer
	// already ensures Limit >= 0 && Limit <= 1000, but we add local protection.
	fetchLimit := filters.Limit + 1
	if fetchLimit <= 0 {
		// This should never happen due to upstream validation, but protect against
		// potential bypass or refactoring. Use default limit + 1 as safe fallback.
		fetchLimit = model.DefaultTransactionValidationFilterLimit + 1
	}

	qb = qb.Limit(uint64(fetchLimit)) // #nosec G115 - fetchLimit is validated positive above

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.transaction_validation.list"),
		libLog.Int("filter.limit", filters.Limit),
		libLog.Bool("filter.has_cursor", filters.Cursor != ""),
	).Log(ctx, libLog.LevelInfo, "Listing transaction validations")

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to list transaction validations", err)
		return nil, fmt.Errorf("failed to list transaction validations: %w", err)
	}
	defer rows.Close()

	var validations []*model.TransactionValidation

	for rows.Next() {
		validation, err := r.scanValidationFromRows(ctx, rows)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to scan transaction validation", err)
			return nil, fmt.Errorf("failed to scan transaction validation: %w", err)
		}

		validations = append(validations, validation)
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Error iterating transaction validations", err)
		return nil, fmt.Errorf("error iterating transaction validations: %w", err)
	}

	// Determine if there are more results
	hasMore := len(validations) > filters.Limit

	if hasMore {
		validations = validations[:filters.Limit]
	}

	// Generate next cursor from the last item
	var nextCursor string

	if hasMore && len(validations) > 0 {
		lastValidation := validations[len(validations)-1]

		nextCursor, err = r.buildNextCursor(lastValidation, sortBy, sortOrder)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to encode cursor", err)
			return nil, fmt.Errorf("failed to encode cursor: %w", err)
		}
	}

	// Ensure we return empty slice, not nil
	if validations == nil {
		validations = []*model.TransactionValidation{}
	}

	result := &model.ListTransactionValidationsResult{
		TransactionValidations: validations,
		NextCursor:             nextCursor,
		HasMore:                hasMore,
	}

	logger.With(
		libLog.String("operation", "repository.transaction_validation.list"),
		libLog.Int("result.count", len(validations)),
		libLog.Bool("result.has_more", hasMore),
	).Log(ctx, libLog.LevelInfo, "Listed transaction validations")

	return result, nil
}

// Count returns the total number of records matching the filters.
// Useful for pagination metadata without fetching all records.
func (r *TransactionValidationRepository) Count(ctx context.Context, filters *model.TransactionValidationFilters) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.transaction_validation.count")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Apply defaults if filters is nil
	if filters == nil {
		filters = &model.TransactionValidationFilters{}
	}

	filters.SetDefaults()

	// Validate filters
	if err := filters.Validate(); err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid transaction validation filters", err)

		return 0, fmt.Errorf("%w: %w", constant.ErrInvalidTransactionValidationFilters, err)
	}

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)

		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	qb := sq.Select("count(*)").
		From(r.tableName).
		PlaceholderFormat(sq.Dollar)

	// Apply filters (excluding pagination for count)
	qb = r.applyFilters(qb, filters)

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)

		return 0, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.transaction_validation.count"),
	).Log(ctx, libLog.LevelInfo, "Counting transaction validations")

	var count int64

	err = db.QueryRowContext(ctx, sqlStr, args...).Scan(&count)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to count transaction validations", err)

		return 0, fmt.Errorf("failed to count transaction validations: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.transaction_validation.count"),
		libLog.Any("result.count", count),
	).Log(ctx, libLog.LevelInfo, "Counted transaction validations")

	return count, nil
}

// applyFilters adds WHERE clauses based on the provided filters.
func (r *TransactionValidationRepository) applyFilters(qb sq.SelectBuilder, filters *model.TransactionValidationFilters) sq.SelectBuilder {
	// Date range filter
	if !filters.StartDate.IsZero() {
		qb = qb.Where(sq.GtOrEq{"created_at": filters.StartDate})
	}

	if !filters.EndDate.IsZero() {
		qb = qb.Where(sq.LtOrEq{"created_at": filters.EndDate})
	}

	// Decision filter
	if filters.Decision != nil {
		qb = qb.Where(sq.Eq{"decision": string(*filters.Decision)})
	}

	// AccountID filter (JSONB path query on account)
	if filters.AccountID != nil {
		qb = qb.Where("account->>'accountId' = ?", filters.AccountID.String())
	}

	// MatchedRuleID filter (ANY on UUID[] array)
	if filters.MatchedRuleID != nil {
		qb = qb.Where("? = ANY(matched_rule_ids)", filters.MatchedRuleID.String())
	}

	// ExceededLimitID filter (JSONB path query on limit_usage_details array)
	if filters.ExceededLimitID != nil {
		qb = qb.Where(
			"EXISTS (SELECT 1 FROM jsonb_array_elements(limit_usage_details) AS lud WHERE lud->>'limitId' = ? AND (lud->>'exceeded')::boolean = true)",
			filters.ExceededLimitID.String(),
		)
	}

	// SegmentID filter (JSONB path query on segment)
	if filters.SegmentID != nil {
		qb = qb.Where("segment->>'segmentId' = ?", filters.SegmentID.String())
	}

	// PortfolioID filter (JSONB path query on portfolio)
	if filters.PortfolioID != nil {
		qb = qb.Where("portfolio->>'portfolioId' = ?", filters.PortfolioID.String())
	}

	// TransactionType filter
	if filters.TransactionType != nil {
		qb = qb.Where(sq.Eq{"transaction_type": string(*filters.TransactionType)})
	}

	return qb
}

// scanValidation scans a single row into a TransactionValidation struct.
// Uses the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func (r *TransactionValidationRepository) scanValidation(ctx context.Context, row *sql.Row) (*model.TransactionValidation, error) {
	var (
		dbModel          TransactionValidationPostgreSQLModel
		segmentJSON      []byte
		portfolioJSON    []byte
		merchantJSON     []byte
		matchedRuleIDs   StringArray
		evaluatedRuleIDs StringArray
	)

	// Temporary variables for nullable JSONB fields
	var accountJSON, metadataJSON, limitUsageDetailsJSON []byte

	// Check for context cancellation before processing
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	err := row.Scan(
		&dbModel.ID,
		&dbModel.RequestID,
		&dbModel.TransactionType,
		&dbModel.SubType,
		&dbModel.Amount,
		&dbModel.Currency,
		&dbModel.TransactionTimestamp,
		&accountJSON,
		&segmentJSON,
		&portfolioJSON,
		&merchantJSON,
		&metadataJSON,
		&dbModel.Decision,
		&dbModel.Reason,
		&matchedRuleIDs,
		&evaluatedRuleIDs,
		&limitUsageDetailsJSON,
		&dbModel.ProcessingTimeMs,
		&dbModel.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Convert byte slices to strings for the model
	dbModel.Account = string(accountJSON)
	dbModel.Metadata = string(metadataJSON)
	dbModel.LimitUsageDetails = string(limitUsageDetailsJSON)

	// Handle nullable JSONB fields
	if len(segmentJSON) > 0 {
		segmentStr := string(segmentJSON)
		dbModel.Segment = &segmentStr
	}

	if len(portfolioJSON) > 0 {
		portfolioStr := string(portfolioJSON)
		dbModel.Portfolio = &portfolioStr
	}

	if len(merchantJSON) > 0 {
		merchantStr := string(merchantJSON)
		dbModel.Merchant = &merchantStr
	}

	// Convert UUID arrays from PostgreSQL format
	dbModel.MatchedRuleIds = formatUUIDArrayFromStringArray(matchedRuleIDs)
	dbModel.EvaluatedRuleIds = formatUUIDArrayFromStringArray(evaluatedRuleIDs)

	// Use ToEntity to convert to domain model
	validation, err := dbModel.ToEntity()
	if err != nil {
		return nil, fmt.Errorf("failed to convert to entity: %w", err)
	}

	return validation, nil
}

// scanValidationFromRows scans a row from sql.Rows into a TransactionValidation struct.
// Uses the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func (r *TransactionValidationRepository) scanValidationFromRows(ctx context.Context, rows *sql.Rows) (*model.TransactionValidation, error) {
	var (
		dbModel          TransactionValidationPostgreSQLModel
		segmentJSON      []byte
		portfolioJSON    []byte
		merchantJSON     []byte
		matchedRuleIDs   StringArray
		evaluatedRuleIDs StringArray
	)

	// Temporary variables for nullable JSONB fields
	var accountJSON, metadataJSON, limitUsageDetailsJSON []byte

	// Check for context cancellation before processing
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	err := rows.Scan(
		&dbModel.ID,
		&dbModel.RequestID,
		&dbModel.TransactionType,
		&dbModel.SubType,
		&dbModel.Amount,
		&dbModel.Currency,
		&dbModel.TransactionTimestamp,
		&accountJSON,
		&segmentJSON,
		&portfolioJSON,
		&merchantJSON,
		&metadataJSON,
		&dbModel.Decision,
		&dbModel.Reason,
		&matchedRuleIDs,
		&evaluatedRuleIDs,
		&limitUsageDetailsJSON,
		&dbModel.ProcessingTimeMs,
		&dbModel.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Convert byte slices to strings for the model
	dbModel.Account = string(accountJSON)
	dbModel.Metadata = string(metadataJSON)
	dbModel.LimitUsageDetails = string(limitUsageDetailsJSON)

	// Handle nullable JSONB fields
	if len(segmentJSON) > 0 {
		segmentStr := string(segmentJSON)
		dbModel.Segment = &segmentStr
	}

	if len(portfolioJSON) > 0 {
		portfolioStr := string(portfolioJSON)
		dbModel.Portfolio = &portfolioStr
	}

	if len(merchantJSON) > 0 {
		merchantStr := string(merchantJSON)
		dbModel.Merchant = &merchantStr
	}

	// Convert UUID arrays from PostgreSQL format
	dbModel.MatchedRuleIds = formatUUIDArrayFromStringArray(matchedRuleIDs)
	dbModel.EvaluatedRuleIds = formatUUIDArrayFromStringArray(evaluatedRuleIDs)

	// Use ToEntity to convert to domain model
	validation, err := dbModel.ToEntity()
	if err != nil {
		return nil, fmt.Errorf("failed to convert to entity: %w", err)
	}

	return validation, nil
}

// validateAndNormalizeSort validates and normalizes sort parameters.
// Returns the validated sortBy, sortOrder values, and any validation error.
func (r *TransactionValidationRepository) validateAndNormalizeSort(filters *model.TransactionValidationFilters) (string, string, error) {
	sortBy := filters.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}

	if !model.IsValidTransactionValidationSortField(sortBy) {
		return "", "", constant.ErrInvalidSortColumn
	}

	// Map API field names to database column names
	if col, ok := sortFieldToColumn[sortBy]; ok {
		sortBy = col
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

	return sortBy, sortOrder, nil
}

// applyCursorFilter adds keyset pagination WHERE clause to the query.
// Supports custom sort columns with id as tiebreaker.
// Returns the updated query, sort column, and sort order from the cursor (for consistency).
func (r *TransactionValidationRepository) applyCursorFilter(qb sq.SelectBuilder, cursorStr string, requestedSortBy string, requestedOrderDir string, span trace.Span) (sq.SelectBuilder, string, string, error) {
	if cursorStr == "" {
		return qb, requestedSortBy, requestedOrderDir, nil
	}

	cursor, err := pkgHTTP.DecodeCursor(cursorStr)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid cursor", err)
		return qb, requestedSortBy, requestedOrderDir, fmt.Errorf("%w: %w", constant.ErrInvalidCursor, err)
	}

	// Use sort column and order from cursor for consistency across pages
	// Cursor stores snake_case column names (already normalized)
	sortColumn := cursor.SortBy
	if sortColumn == "" {
		sortColumn = "created_at"
	} else if !validTransactionValidationDBColumns[sortColumn] {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid sort column in cursor", constant.ErrInvalidSortColumn)
		return qb, requestedSortBy, requestedOrderDir, constant.ErrInvalidSortColumn
	}

	orderDir := cursor.SortOrder
	if orderDir != "ASC" && orderDir != "DESC" {
		orderDir = "DESC"
	}

	// Validate cursor sort parameters match request parameters
	// This prevents clients from changing sort mid-pagination which could cause inconsistent results
	if sortColumn != requestedSortBy {
		libOtel.HandleSpanBusinessErrorEvent(span, "Cursor sort mismatch", constant.ErrInvalidCursor)
		return qb, requestedSortBy, requestedOrderDir, fmt.Errorf("%w: cursor sortBy does not match request", constant.ErrInvalidCursor)
	}

	if orderDir != strings.ToUpper(requestedOrderDir) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Cursor sort order mismatch", constant.ErrInvalidCursor)
		return qb, requestedSortBy, requestedOrderDir, fmt.Errorf("%w: cursor sortOrder does not match request", constant.ErrInvalidCursor)
	}

	// Validate cursor sort value type matches expected column type
	if err := validateCursorSortValueTransactionValidation(sortColumn, cursor.SortValue); err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid cursor sort value type", constant.ErrInvalidCursor)
		return qb, requestedSortBy, requestedOrderDir, fmt.Errorf("%w: %w", constant.ErrInvalidCursor, err)
	}

	// Build WHERE clause based on sort column
	qb = r.buildCursorCondition(qb, &cursor, sortColumn, orderDir)

	return qb, sortColumn, orderDir, nil
}

// buildCursorCondition builds WHERE clause for keyset pagination.
// Uses sort value + ID as tiebreaker for consistent pagination.
func (r *TransactionValidationRepository) buildCursorCondition(qb sq.SelectBuilder, cursor *pkgHTTP.Cursor, sortBy, sortOrder string) sq.SelectBuilder {
	lt := sq.Lt{}
	gt := sq.Gt{}
	eq := sq.Eq{}

	lt[sortBy] = cursor.SortValue
	gt[sortBy] = cursor.SortValue
	eq[sortBy] = cursor.SortValue

	if sortOrder == "DESC" {
		return qb.Where(
			sq.Or{
				lt, // sort_value < cursor
				sq.And{
					eq,                     // sort_value = cursor
					sq.Lt{"id": cursor.ID}, // AND id < cursor
				},
			},
		)
	}

	return qb.Where(
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
// sortBy is validated against validSortFields whitelist before calling this method.
// sortOrder is constrained to "ASC" or "DESC" before calling this method.
func (r *TransactionValidationRepository) applyOrderBy(qb sq.SelectBuilder, sortBy, sortOrder string) sq.SelectBuilder {
	// Use string concatenation instead of fmt.Sprintf
	// sortBy and sortOrder are pre-validated by whitelist and constraint checks
	return qb.OrderBy(sortBy + " " + sortOrder + ", id " + sortOrder)
}

// buildNextCursor creates a base64-encoded cursor from the last validation in the result set.
// Validates sortBy against allowed database columns and normalizes sortOrder to uppercase.
// Note: sortBy is expected to be snake_case (already normalized by validateAndNormalizeSort).
func (r *TransactionValidationRepository) buildNextCursor(validation *model.TransactionValidation, sortBy, sortOrder string) (string, error) {
	// Validate sortBy against database column whitelist (snake_case)
	if !validTransactionValidationDBColumns[sortBy] {
		return "", constant.ErrInvalidSortColumn
	}

	// Normalize sortOrder to uppercase
	normalizedSortOrder := strings.ToUpper(sortOrder)
	if normalizedSortOrder != "ASC" && normalizedSortOrder != "DESC" {
		normalizedSortOrder = "DESC"
	}

	sortValue, err := getSortValueFromValidation(validation, sortBy)
	if err != nil {
		return "", fmt.Errorf("failed to get sort value: %w", err)
	}

	cursor := pkgHTTP.Cursor{
		ID:         validation.ID.String(),
		SortValue:  sortValue,
		SortBy:     sortBy,
		SortOrder:  normalizedSortOrder,
		PointsNext: true,
	}

	return pkgHTTP.EncodeCursor(cursor)
}

// validateCursorSortValueTransactionValidation validates that the cursor sort value has the correct type
// for the given sort column. This prevents database type coercion errors and
// unexpected query results.
func validateCursorSortValueTransactionValidation(sortBy, sortValue string) error {
	switch sortBy {
	case "created_at":
		// Timestamp columns expect RFC3339Nano format
		if _, err := time.Parse(time.RFC3339Nano, sortValue); err != nil {
			return fmt.Errorf("invalid timestamp format for %s", sortBy)
		}
	case "processing_time_ms":
		// Float column - parse as float64 (sub-millisecond precision)
		// Reject non-finite values (NaN, Inf) and hex float literals
		v, err := strconv.ParseFloat(sortValue, 64)
		if err != nil {
			return fmt.Errorf("invalid float format for %s", sortBy)
		}

		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("invalid float format for %s: non-finite value", sortBy)
		}

		if strconv.FormatFloat(v, 'f', -1, 64) != sortValue {
			return fmt.Errorf("invalid float format for %s: non-decimal literal", sortBy)
		}
	default:
		return fmt.Errorf("unsupported sort column: %s", sortBy)
	}

	return nil
}

// getSortValueFromValidation extracts the value of the sort column from a validation.
// Returns an error if sortBy is not a supported column.
func getSortValueFromValidation(validation *model.TransactionValidation, sortBy string) (string, error) {
	switch sortBy {
	case "created_at":
		return validation.CreatedAt.Format(time.RFC3339Nano), nil
	case "processing_time_ms":
		return strconv.FormatFloat(validation.ProcessingTimeMs, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("unsupported sort column: %s", sortBy)
	}
}

// uuidSliceToStringArray converts a slice of UUIDs to StringArray for PostgreSQL UUID[] type.
// StringArray implements driver.Value interface required for database/sql compatibility.
func uuidSliceToStringArray(uuids []uuid.UUID) StringArray {
	if uuids == nil {
		return StringArray{}
	}

	result := make(StringArray, len(uuids))
	for i, id := range uuids {
		result[i] = id.String()
	}

	return result
}

// formatStringArrayToPostgres formats a slice of UUID strings to PostgreSQL array format "{item1,item2,...}".
// IMPORTANT: This function does NOT escape special characters. Only use with
// validated UUID strings that cannot contain commas, braces, or quotes.
func formatStringArrayToPostgres(strs []string) string {
	if len(strs) == 0 {
		return "{}"
	}

	result := "{"

	for i, s := range strs {
		if i > 0 {
			result += ","
		}

		result += s
	}

	result += "}"

	return result
}

// formatUUIDArrayFromStringArray converts a StringArray to PostgreSQL UUID array string format.
// Used by scanValidation and scanValidationFromRows to prepare data for ToEntity conversion.
// Format: "{uuid1,uuid2,...}" or "{}" for empty arrays.
func formatUUIDArrayFromStringArray(strs StringArray) string {
	return formatStringArrayToPostgres([]string(strs))
}
