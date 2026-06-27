// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	_ query.AuditEventRepository   = (*AuditEventRepository)(nil)
	_ command.AuditEventRepository = (*AuditEventRepository)(nil)
)

// AuditEventRepository implements audit event persistence using PostgreSQL.
// Tenant resolution is handled by the underlying pgdb.Connection (M1).
type AuditEventRepository struct {
	conn      pgdb.Connection
	tableName string
}

// NewAuditEventRepositoryWithConnection creates a new PostgreSQL audit event repository with a custom pgdb.Connection.
// This is primarily used for testing with mock connections.
func NewAuditEventRepositoryWithConnection(conn pgdb.Connection) *AuditEventRepository {
	return &AuditEventRepository{
		conn:      conn,
		tableName: "audit_events",
	}
}

// Insert creates a new audit event record (append-only).
func (r *AuditEventRepository) Insert(ctx context.Context, event *model.AuditEvent) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.audit_event.insert")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	return r.insertInternal(ctx, db, event, logger, span, "repository.audit_event.insert")
}

// InsertWithTx creates a new audit event record using the provided database connection.
// This allows callers to pass either a regular DB connection or a transaction (*sql.Tx),
// enabling atomic operations with other database changes.
func (r *AuditEventRepository) InsertWithTx(ctx context.Context, db pgdb.DB, event *model.AuditEvent) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	if db == nil {
		return pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.audit_event.insert_with_tx")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	return r.insertInternal(ctx, db, event, logger, span, "repository.audit_event.insert_with_tx")
}

// insertInternal contains the shared logic for Insert and InsertWithTx.
// It performs marshaling, query building, and execution using the provided database connection.
func (r *AuditEventRepository) insertInternal(
	ctx context.Context,
	db pgdb.DB,
	event *model.AuditEvent,
	logger libLog.Logger,
	span trace.Span,
	operationName string,
) error {
	contextJSON, err := json.Marshal(event.Context)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to marshal context", err)
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	metadataJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to marshal metadata", err)
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Deduplication for transaction validation events:
	// The partial unique index idx_audit_events_validation_dedup ensures only the first
	// audit event per (resource_id, event_type) is stored when resource_type = 'transaction'.
	//
	// We use INSERT...SELECT...WHERE NOT EXISTS instead of ON CONFLICT DO NOTHING because
	// audit_events has PostgreSQL RULEs (prevent_audit_event_update, prevent_audit_event_delete)
	// and PostgreSQL does not allow ON CONFLICT on tables with RULEs.
	//
	// For non-transaction resource types, the condition ($17 = 'transaction') is false,
	// so the WHERE NOT EXISTS clause is bypassed and the INSERT always proceeds.
	sqlStr := `
		INSERT INTO audit_events (
			event_id, event_type, created_at, action, result,
			resource_id, resource_type,
			actor_type, actor_id, actor_name, actor_role, actor_ip_address,
			context, metadata
		)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14::jsonb
		WHERE NOT EXISTS (
			SELECT 1 FROM audit_events
			WHERE resource_id = $15
			  AND event_type = $16
			  AND resource_type = 'transaction'
			  AND $17 = 'transaction'
		)
	`
	args := []any{
		event.EventID, string(event.EventType), event.CreatedAt,
		string(event.Action), string(event.Result),
		event.ResourceID, string(event.ResourceType),
		string(event.Actor.ActorType), event.Actor.ID, event.Actor.Name,
		nullableString(event.Actor.Role), event.Actor.IPAddress,
		contextJSON, metadataJSON,
		event.ResourceID, string(event.EventType), string(event.ResourceType),
	}

	logger.With(
		libLog.String("operation", operationName),
		libLog.String("event.id", event.EventID.String()),
		libLog.String("event.type", string(event.EventType)),
	).Log(ctx, libLog.LevelDebug, "Inserting audit event record")

	result, err := db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to insert audit event", err)
		return fmt.Errorf("failed to insert audit event: %w", err)
	}

	// Log dedup visibility for transaction validation events
	if event.ResourceType == model.ResourceTypeTransaction {
		rowsAffected, rowsErr := result.RowsAffected()
		if rowsErr == nil && rowsAffected == 0 {
			logger.With(
				libLog.String("event.id", event.EventID.String()),
				libLog.String("resource.id", event.ResourceID),
				libLog.String("event.type", string(event.EventType)),
			).Log(ctx, libLog.LevelDebug, "Audit event skipped due to deduplication")
		}
	}

	return nil
}

// GetByID retrieves an audit event by its event ID.
func (r *AuditEventRepository) GetByID(ctx context.Context, eventID uuid.UUID) (*model.AuditEvent, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.audit_event.get_by_event_id")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	qb := r.baseSelectBuilder().
		Where(sq.Eq{"event_id": eventID}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.audit_event.get_by_event_id"),
		libLog.String("event.id", eventID.String()),
	).Log(ctx, libLog.LevelDebug, "Getting audit event by event ID")

	event, err := r.scanEvent(db.QueryRowContext(ctx, sqlStr, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Audit event not found", constant.ErrAuditEventNotFound)
			return nil, constant.ErrAuditEventNotFound
		}

		libOtel.HandleSpanError(span, "Failed to get audit event", err)

		return nil, fmt.Errorf("failed to get audit event: %w", err)
	}

	return event, nil
}

// List retrieves audit events with filters and pagination.
func (r *AuditEventRepository) List(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.audit_event.list")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if filters == nil {
		filters = &model.AuditEventFilters{}
	}

	filters.SetDefaults()

	if err := filters.Validate(); err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid audit event filters", err)
		return nil, fmt.Errorf("%w: %w", constant.ErrInvalidAuditEventFilters, err)
	}

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	qb := r.baseSelectBuilder().PlaceholderFormat(sq.Dollar)
	qb = r.applyFilters(qb, filters)

	qb, err = r.applyCursorPagination(qb, filters)
	if err != nil {
		return nil, err
	}

	sortOrder := filters.SortOrder
	sortBy := r.mapSortField(filters.SortBy)
	qb = qb.OrderBy(sortBy + " " + sortOrder + ", id " + sortOrder)

	fetchLimit := filters.Limit + 1
	// Defense-in-depth: ensure fetchLimit is positive before uint64 conversion
	// to prevent integer overflow (gosec G115). Validation at upstream layers
	// already ensures Limit >= 0, but we add local protection.
	if fetchLimit <= 0 {
		// This should never happen due to upstream validation, but protect against
		// potential bypass or refactoring. Use default limit + 1 as safe fallback.
		fetchLimit = model.DefaultAuditEventFilterLimit + 1
	}

	qb = qb.Limit(uint64(fetchLimit)) // #nosec G115 - fetchLimit validated positive above

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build query", err)
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.audit_event.list"),
		libLog.Int("filter.limit", filters.Limit),
	).Log(ctx, libLog.LevelDebug, "Listing audit events")

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to list audit events", err)
		return nil, fmt.Errorf("failed to list audit events: %w", err)
	}
	defer rows.Close()

	var events []*model.AuditEvent

	for rows.Next() {
		event, err := r.scanEventFromRows(rows)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to scan audit event", err)
			return nil, fmt.Errorf("failed to scan audit event: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		libOtel.HandleSpanError(span, "Error iterating audit events", err)
		return nil, fmt.Errorf("error iterating audit events: %w", err)
	}

	hasMore := len(events) > filters.Limit
	if hasMore {
		events = events[:filters.Limit]
	}

	nextCursor, err := r.generateNextCursor(ctx, events, hasMore, filters)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to generate next cursor", err)
		return nil, fmt.Errorf("failed to generate next cursor: %w", err)
	}

	if events == nil {
		events = []*model.AuditEvent{}
	}

	return &model.ListAuditEventsResult{
		AuditEvents: events,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
	}, nil
}

// VerifyHashChain verifies the integrity of the hash chain.
func (r *AuditEventRepository) VerifyHashChain(ctx context.Context, eventID uuid.UUID) (*model.HashChainVerificationResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.audit_event.verify_hash_chain")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get database connection", err)
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// First, get the internal ID for this event
	var internalID int64

	err = db.QueryRowContext(ctx,
		"SELECT id FROM audit_events WHERE event_id = $1",
		eventID,
	).Scan(&internalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, constant.ErrAuditEventNotFound
		}

		return nil, fmt.Errorf("failed to get event internal ID: %w", err)
	}

	// Call the verification function
	var (
		isValid        bool
		firstInvalidID sql.NullInt64
		totalChecked   int64
		errorDetail    sql.NullString
	)

	err = db.QueryRowContext(ctx,
		"SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, $1)",
		internalID,
	).Scan(&isValid, &firstInvalidID, &totalChecked, &errorDetail)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to verify hash chain", err)
		return nil, fmt.Errorf("failed to verify hash chain: %w", err)
	}

	logger.With(
		libLog.String("operation", "repository.audit_event.verify_hash_chain"),
		libLog.String("event.id", eventID.String()),
		libLog.Bool("is_valid", isValid),
		libLog.Any("total_checked", totalChecked),
	).Log(ctx, libLog.LevelDebug, "Hash chain verification completed")

	result := &model.HashChainVerificationResult{
		IsValid:      isValid,
		TotalChecked: totalChecked,
	}

	if isValid {
		result.Message = "Hash chain integrity verified successfully"
	} else {
		if firstInvalidID.Valid {
			result.FirstInvalidID = &firstInvalidID.Int64
		}

		result.Message = "Hash chain integrity compromised - possible data tampering detected"
	}

	return result, nil
}

// Helper methods

func (r *AuditEventRepository) baseSelectBuilder() sq.SelectBuilder {
	return sq.Select(
		"id", "hash", "previous_hash",
		"event_id", "event_type", "created_at", "action", "result",
		"resource_id", "resource_type",
		"actor_type", "actor_id", "actor_name", "actor_role", "actor_ip_address",
		"context", "metadata",
	).From(r.tableName)
}

func (r *AuditEventRepository) applyCursorPagination(qb sq.SelectBuilder, filters *model.AuditEventFilters) (sq.SelectBuilder, error) {
	if filters.Cursor == "" {
		return qb, nil
	}

	cursor, err := pkgHTTP.DecodeCursor(filters.Cursor)
	if err != nil {
		return qb, fmt.Errorf("%w: %w", constant.ErrInvalidCursor, err)
	}

	// Validate that cursor sort parameters match request filters
	if cursor.SortBy != filters.SortBy {
		return qb, fmt.Errorf("%w: sortBy mismatch (cursor has %q, request has %q)", constant.ErrInvalidCursor, cursor.SortBy, filters.SortBy)
	}

	if cursor.SortOrder != filters.SortOrder {
		return qb, fmt.Errorf("%w: sortOrder mismatch (cursor has %q, request has %q)", constant.ErrInvalidCursor, cursor.SortOrder, filters.SortOrder)
	}

	var cursorID int64

	_, parseErr := fmt.Sscanf(cursor.ID, "%d", &cursorID)
	if parseErr != nil {
		return qb, fmt.Errorf("%w: invalid ID format", constant.ErrInvalidCursor)
	}

	// Map the sort field to the database column name
	sortField := r.mapSortField(filters.SortBy)

	// Use tuple-style comparison: (sortField, id) to maintain stable ordering
	// This ensures no rows are skipped even when sortField values are duplicated
	if filters.SortOrder == "DESC" {
		// For DESC: (sortField < cursor.SortValue) OR (sortField = cursor.SortValue AND id < cursor.ID)
		qb = qb.Where(sq.Or{
			sq.Lt{sortField: cursor.SortValue},
			sq.And{
				sq.Eq{sortField: cursor.SortValue},
				sq.Lt{"id": cursorID},
			},
		})
	} else {
		// For ASC: (sortField > cursor.SortValue) OR (sortField = cursor.SortValue AND id > cursor.ID)
		qb = qb.Where(sq.Or{
			sq.Gt{sortField: cursor.SortValue},
			sq.And{
				sq.Eq{sortField: cursor.SortValue},
				sq.Gt{"id": cursorID},
			},
		})
	}

	return qb, nil
}

func (r *AuditEventRepository) generateNextCursor(ctx context.Context, events []*model.AuditEvent, hasMore bool, filters *model.AuditEventFilters) (string, error) {
	if !hasMore || len(events) == 0 {
		return "", nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "repository.audit_event.generate_next_cursor")
	defer span.End()

	lastEvent := events[len(events)-1]

	// Extract the sort field value from the last event
	sortValue := r.extractSortValue(lastEvent, filters.SortBy)

	cursor := pkgHTTP.Cursor{
		ID:         fmt.Sprintf("%d", lastEvent.ID),
		SortValue:  sortValue,
		SortBy:     filters.SortBy,
		SortOrder:  filters.SortOrder,
		PointsNext: true,
	}

	token, err := pkgHTTP.EncodeCursor(cursor)
	if err != nil {
		logger.With(
			libLog.String("operation", "repository.audit_event.generate_next_cursor"),
			libLog.String("cursor.id", cursor.ID),
			libLog.String("cursor.sort_value", cursor.SortValue),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to encode pagination cursor")

		return "", fmt.Errorf("failed to encode cursor: %w", err)
	}

	return token, nil
}

func (r *AuditEventRepository) applyFilters(qb sq.SelectBuilder, f *model.AuditEventFilters) sq.SelectBuilder {
	if !f.StartDate.IsZero() {
		qb = qb.Where(sq.GtOrEq{"created_at": f.StartDate})
	}

	if !f.EndDate.IsZero() {
		qb = qb.Where(sq.LtOrEq{"created_at": f.EndDate})
	}

	if f.EventType != nil {
		qb = qb.Where(sq.Eq{"event_type": string(*f.EventType)})
	}

	if f.Action != nil {
		qb = qb.Where(sq.Eq{"action": string(*f.Action)})
	}

	if f.Result != nil {
		qb = qb.Where(sq.Eq{"result": string(*f.Result)})
	}

	if f.ResourceType != nil {
		qb = qb.Where(sq.Eq{"resource_type": string(*f.ResourceType)})
	}

	if f.ResourceID != nil {
		qb = qb.Where(sq.Eq{"resource_id": *f.ResourceID})
	}

	if f.ActorType != nil {
		qb = qb.Where(sq.Eq{"actor_type": string(*f.ActorType)})
	}

	if f.ActorID != nil {
		qb = qb.Where(sq.Eq{"actor_id": *f.ActorID})
	}
	// JSONB filters for validation-specific fields (in context.request.account)
	if f.AccountID != nil {
		qb = qb.Where("context->'request'->'account'->>'id' = ?", f.AccountID.String())
	}

	if f.SegmentID != nil {
		qb = qb.Where("context->'request'->'account'->>'segmentId' = ?", f.SegmentID.String())
	}

	if f.PortfolioID != nil {
		qb = qb.Where("context->'request'->'account'->>'portfolioId' = ?", f.PortfolioID.String())
	}

	if f.TransactionType != nil {
		qb = qb.Where("context->'request'->>'transactionType' = ?", string(*f.TransactionType))
	}

	if f.MatchedRuleID != nil {
		qb = qb.Where("context->'response'->'matchedRuleIds' @> ?", fmt.Sprintf("[\"%s\"]", f.MatchedRuleID.String()))
	}

	return qb
}

func (r *AuditEventRepository) mapSortField(field string) string {
	switch field {
	case "created_at":
		return "created_at"
	case "event_type":
		return "event_type"
	default:
		return ""
	}
}

// extractSortValue extracts the value of the sort field from an audit event
func (r *AuditEventRepository) extractSortValue(event *model.AuditEvent, sortBy string) string {
	mappedField := r.mapSortField(sortBy)
	switch mappedField {
	case "created_at":
		return event.CreatedAt.Format(time.RFC3339Nano)
	case "event_type":
		return string(event.EventType)
	default:
		return ""
	}
}

// scanner interface for both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanEventCommon extracts the common scan logic for both scanEvent and scanEventFromRows.
func (r *AuditEventRepository) scanEventCommon(s scanner) (*model.AuditEvent, error) {
	var (
		event        model.AuditEvent
		contextJSON  []byte
		metadataJSON []byte
		actorType    string
		actorRole    sql.NullString
		actorIP      string
		eventType    string
		action       string
		result       string
		resourceType string
		previousHash sql.NullString
	)

	err := s.Scan(
		&event.ID, &event.Hash, &previousHash,
		&event.EventID, &eventType, &event.CreatedAt, &action, &result,
		&event.ResourceID, &resourceType,
		&actorType, &event.Actor.ID, &event.Actor.Name, &actorRole, &actorIP,
		&contextJSON, &metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	return r.hydrateEvent(&event, eventType, action, result, resourceType, actorType,
		actorRole, actorIP, previousHash, contextJSON, metadataJSON)
}

func (r *AuditEventRepository) scanEvent(row *sql.Row) (*model.AuditEvent, error) {
	return r.scanEventCommon(row)
}

func (r *AuditEventRepository) scanEventFromRows(rows *sql.Rows) (*model.AuditEvent, error) {
	return r.scanEventCommon(rows)
}

func (r *AuditEventRepository) hydrateEvent(
	event *model.AuditEvent,
	eventType, action, result, resourceType, actorType string,
	actorRole sql.NullString, actorIP string,
	previousHash sql.NullString,
	contextJSON, metadataJSON []byte,
) (*model.AuditEvent, error) {
	event.EventType = model.AuditEventType(eventType)
	event.Action = model.AuditAction(action)
	event.Result = model.AuditResult(result)
	event.ResourceType = model.ResourceType(resourceType)
	event.Actor.ActorType = model.ActorType(actorType)
	event.Actor.IPAddress = actorIP

	if actorRole.Valid {
		event.Actor.Role = actorRole.String
	}

	if previousHash.Valid {
		event.PreviousHash = previousHash.String
	}

	if len(contextJSON) > 0 {
		if err := json.Unmarshal(contextJSON, &event.Context); err != nil {
			return nil, fmt.Errorf("failed to unmarshal context: %w", err)
		}
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &event.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	if event.Context == nil {
		event.Context = make(map[string]any)
	}

	if event.Metadata == nil {
		event.Metadata = make(map[string]any)
	}

	return event, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}

	return s
}
