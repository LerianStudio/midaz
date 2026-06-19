// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package audit

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// auditCollection is the single shared collection for protection audit events.
// It mirrors the encryption adapter's single-collection convention (not a
// per-organization holders_{org} pattern).
const auditCollection = "protection_audit_log"

// defaultFindLimit and maxFindLimit bound the page size for FindByOrganization.
const (
	defaultFindLimit = 20
	maxFindLimit     = 100
)

// tinkEnvelopeMarker is the literal marker prefix this codebase produces for
// envelope-encrypted values. It is kept as a local literal (rather than importing
// the encryption service package) to avoid an import cycle between the audit
// adapter and the encryption service. It MUST stay in sync with
// components/crm/internal/services/encryption/marker.go (MarkerPrefix = "tink:v").
const tinkEnvelopeMarker = "tink:v"

// forbiddenContentPatterns are regexes for secrets/keysets that MUST NOT be
// persisted into the audit log even by accident. They are compiled once at
// package init. The patterns intentionally never capture the offending value
// for logging; only the pattern name is ever surfaced.
//
//nolint:gochecknoglobals // compiled-once package-level regexes
var forbiddenContentPatterns = map[string]*regexp.Regexp{
	// Vault token, e.g. "hvs.CAESIJ8x_abc-123".
	"vault_token": regexp.MustCompile(`hvs\.[A-Za-z0-9_-]+`),
	// Tink keyset JSON marker, e.g. `"primaryKeyId": 123456789`.
	"tink_keyset_marker": regexp.MustCompile(`"primaryKeyId"\s*:\s*\d+`),
	// Tink envelope marker, e.g. "tink:v1:{url-safe base64}". Aligned with
	// encryption.MarkerPrefix ("tink:v"); see marker.go FormatEnvelopeMarker.
	"tink_envelope_marker": regexp.MustCompile(regexp.QuoteMeta(tinkEnvelopeMarker) + `\d+:[A-Za-z0-9+/=_-]+`),
}

// Repository persists and queries protection audit events. Audit events are
// append-only: there are intentionally no Update or Delete operations.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=audit.mongodb_mock.go --package=audit . Repository
type Repository interface {
	// Create appends a single protection audit event.
	Create(ctx context.Context, event *mmodel.ProtectionAuditEvent) error
	// FindByOrganization returns audit events for an organization filtered by
	// query, along with opaque next/prev pagination cursors. The cursors are
	// empty when there is no further page in the corresponding direction.
	FindByOrganization(ctx context.Context, organizationID string, query AuditQuery) ([]*mmodel.ProtectionAuditEvent, libHTTP.CursorPagination, error)
}

// AuditQuery carries the filter, ordering, and cursor-pagination parameters for
// FindByOrganization. A zero StartTime or EndTime means the bound is unset.
type AuditQuery struct {
	Limit     int
	Cursor    string
	SortOrder string
	Action    string
	Actor     string
	Outcome   string
	StartTime time.Time
	EndTime   time.Time
}

// MongoDBRepository is a MongoDB-specific implementation of Repository.
type MongoDBRepository struct {
	connection *libMongo.Client
}

// NewMongoDBRepository returns a new instance of MongoDBRepository using the given MongoDB connection.
// In multi-tenant mode, connection may be nil — the per-request tenant context provides the database.
func NewMongoDBRepository(connection *libMongo.Client) (*MongoDBRepository, error) {
	r := &MongoDBRepository{
		connection: connection,
	}

	if connection != nil {
		if _, err := r.connection.Database(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to connect to MongoDB for audit repository: %w", err)
		}
	}

	return r, nil
}

// Create appends a single protection audit event.
//
// Audit writes are best-effort and non-blocking: the returned error exists for
// testability, but callers MUST treat a failure as a warning and never let it
// block the parent operation. The event never carries plaintext, keysets,
// wrapped keysets, DEK/KEK material, credentials, Vault tokens, or PII; a
// forbidden-content guard drops (without inserting) any event whose free-text
// fields match a secret/keyset pattern.
func (r *MongoDBRepository) Create(ctx context.Context, event *mmodel.ProtectionAuditEvent) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.audit.create")
	defer span.End()

	if event == nil {
		err := pkg.ValidateBusinessError(constant.ErrAuditWriteFailed, constant.EntityProtectionAuditEvent)
		libOpenTelemetry.HandleSpanError(span, "audit event is nil", err)

		return err
	}

	span.SetAttributes(
		attribute.String("app.request.organization_id", event.OrganizationID),
		attribute.String("app.request.event_type", string(event.EventType)),
		attribute.String("app.request.action", string(event.Action)),
		attribute.String("app.request.outcome", string(event.Outcome)),
	)

	// Forbidden-content guard: drop + warn, never insert, never log the value.
	if containsForbiddenContent(event) {
		err := pkg.ValidateBusinessError(constant.ErrAuditWriteFailed, constant.EntityProtectionAuditEvent)

		logger.Log(ctx, libLog.LevelWarn, "audit event rejected: forbidden content", safeLogFields(event)...)
		libOpenTelemetry.HandleSpanError(span, "audit event rejected: forbidden content", err)

		return err
	}

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)

		return err
	}

	if err := r.ensureIndexes(ctx, collection); err != nil {
		// Index creation is part of the best-effort write contract: warn + drop,
		// surfacing an error that satisfies errors.Is(err, ErrAuditWriteFailed),
		// consistent with the insert/guard branches.
		logger.Log(ctx, libLog.LevelWarn, "audit indexes not ensured", safeLogFields(event)...)
		libOpenTelemetry.HandleSpanError(span, "Failed to create audit indexes", err)

		return fmt.Errorf("create audit indexes for %q: %w", auditCollection, errors.Join(constant.ErrAuditWriteFailed, err))
	}

	model := FromEntity(event)

	if _, err := collection.InsertOne(ctx, model); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "audit event insert failed", safeLogFields(event)...)
		libOpenTelemetry.HandleSpanError(span, "Failed to insert audit event", err)

		return fmt.Errorf("insert audit event into %q: %w", auditCollection, errors.Join(constant.ErrAuditWriteFailed, err))
	}

	return nil
}

// containsForbiddenContent reports whether any free-text/string field of the
// event matches a forbidden secret/keyset pattern. It is pure (no I/O) so it is
// fully unit-testable, and it deliberately scans only the fields that could
// leak operator free-text or provider data: Reason and the four string fields
// of Details (guarding a nil Details). It NEVER returns or logs the offending
// value — only a boolean.
func containsForbiddenContent(event *mmodel.ProtectionAuditEvent) bool {
	if event == nil {
		return false
	}

	candidates := []string{event.Reason}

	if event.Details != nil {
		candidates = append(candidates,
			event.Details.ProviderReference,
			event.Details.ErrorCode,
			event.Details.PreviousStatus,
			event.Details.NewStatus,
		)
	}

	haystack := strings.Join(candidates, "\n")

	for _, pattern := range forbiddenContentPatterns {
		if pattern.MatchString(haystack) {
			return true
		}
	}

	return false
}

// safeLogFields converts the event's safe-to-log fields into structured log
// fields. It relies on ProtectionAuditEvent.SafeLogFields, which excludes
// Reason and any other field that could carry sensitive free-text.
func safeLogFields(event *mmodel.ProtectionAuditEvent) []libLog.Field {
	safe := event.SafeLogFields()

	fields := make([]libLog.Field, 0, len(safe))
	for key, value := range safe {
		fields = append(fields, libLog.Any(key, value))
	}

	return fields
}

// FindByOrganization returns audit events for an organization filtered by query.
//
// Pagination is cursor-based on _id: because event IDs are UUIDv7 (chronological),
// _id ordering matches time ordering. It adopts the lib-commons cursor toolkit
// (DecodeCursor/CursorDirectionRules/PaginateRecords/CalculateCursor) so cursors
// are opaque base64 tokens carrying BOTH next and prev navigation. Results
// default to newest-first (descending) and are bounded to a max page size. An
// unparseable cursor is rejected with libHTTP.ErrInvalidCursor rather than
// silently ignored.
func (r *MongoDBRepository) FindByOrganization(ctx context.Context, organizationID string, query AuditQuery) ([]*mmodel.ProtectionAuditEvent, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.audit.find")
	defer span.End()

	// Safe attributes only: org id, paging shape, and which filters are set —
	// never the filter VALUES (action/actor/outcome), which could carry context
	// not appropriate for telemetry.
	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID),
		attribute.Int("app.request.limit", query.Limit),
		attribute.String("app.request.sort_order", query.SortOrder),
		attribute.Bool("app.request.filter_action", query.Action != ""),
		attribute.Bool("app.request.filter_actor", query.Actor != ""),
		attribute.Bool("app.request.filter_outcome", query.Outcome != ""),
		attribute.Bool("app.request.filter_start_time", !query.StartTime.IsZero()),
		attribute.Bool("app.request.filter_end_time", !query.EndTime.IsZero()),
	)

	// Compute the clamped page size exactly once and reuse it for both the
	// Mongo over-fetch (limit+1) and the lib-commons paginate/cursor trimming.
	limit := clampLimit(query.Limit)

	filter := buildFindFilter(organizationID, query)

	isFirstPage := query.Cursor == ""

	cursorDirection := libHTTP.CursorDirectionNext

	if !isFirstPage {
		decoded, err := libHTTP.DecodeCursor(query.Cursor)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "invalid audit pagination cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		cursorDirection = decoded.Direction

		// Derive the comparison operator and effective sort order from the
		// requested sort order + the cursor's navigation direction.
		operator, _, err := libHTTP.CursorDirectionRules(normalizeSortOrder(query.SortOrder), cursorDirection)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "invalid audit cursor direction", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		applyCursorComparison(filter, mongoOperatorFor(operator), decoded.ID)
	}

	// Effective Mongo sort: derive from the cursor rules so prev/next pages scan
	// in the direction that keeps the _id range and the IXSCAN aligned. On the
	// first page there is no cursor, so fall back to the requested order.
	ascending := r.effectiveAscending(query.SortOrder, isFirstPage, cursorDirection)

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	cursor, err := collection.Find(ctx, filter, findOptions(limit, ascending))
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to query audit events", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("query audit events from %q: %w", auditCollection, err)
	}

	defer func() {
		if closeErr := cursor.Close(ctx); closeErr != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to close audit cursor", closeErr)
		}
	}()

	results := make([]*mmodel.ProtectionAuditEvent, 0, limit+1)

	for cursor.Next(ctx) {
		var model AuditEventModel
		if decodeErr := cursor.Decode(&model); decodeErr != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to decode audit event", decodeErr)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("decode audit event from %q: %w", auditCollection, decodeErr)
		}

		results = append(results, model.ToEntity())
	}

	if err := cursor.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to iterate audit events", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("iterate audit events from %q: %w", auditCollection, err)
	}

	hasPagination := int64(len(results)) > limit

	events := libHTTP.PaginateRecords(isFirstPage, hasPagination, cursorDirection, results, int(limit))

	pagination := libHTTP.CursorPagination{}

	if len(events) > 0 {
		pagination, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, cursorDirection, events[0].ID.String(), events[len(events)-1].ID.String())
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	span.SetAttributes(attribute.Int("db.rows_returned", len(events)))

	logger.Log(ctx, libLog.LevelDebug, "audit events queried",
		libLog.Int("rows_returned", len(events)),
		libLog.Bool("has_next_page", pagination.Next != ""),
		libLog.Bool("has_prev_page", pagination.Prev != ""),
	)

	return events, pagination, nil
}

// buildFindFilter builds the MongoDB query filter for FindByOrganization. The
// organization_id equality is always present; action/actor_id/outcome equalities
// and the timestamp range bounds are added only when set. Tenant scoping is
// handled by database resolution (see getDatabase), so no tenant_id filter is
// added here. The cursor _id comparison is applied separately by
// applyCursorComparison after the opaque cursor token is decoded. All values are
// placed via direct field equality, never string-concatenated, to avoid query
// injection.
func buildFindFilter(organizationID string, query AuditQuery) bson.M {
	filter := bson.M{"organization_id": organizationID}

	if query.Action != "" {
		filter["action"] = query.Action
	}

	if query.Actor != "" {
		filter["actor_id"] = query.Actor
	}

	if query.Outcome != "" {
		filter["outcome"] = query.Outcome
	}

	if timeRange := timestampRange(query); timeRange != nil {
		filter["timestamp"] = timeRange
	}

	return filter
}

// applyCursorComparison adds the keyset _id range predicate to an existing
// filter. mongoOp is the MongoDB comparison operator ("$gt"/"$lt") derived from
// the lib-commons cursor rules; cursorID is the decoded cursor's _id. Because
// _id is UUIDv7 (chronological), this range predicate plus the _id sort yields a
// single IXSCAN with no in-memory sort.
func applyCursorComparison(filter bson.M, mongoOp, cursorID string) {
	filter["_id"] = bson.M{mongoOp: cursorID}
}

// mongoOperatorFor maps the lib-commons cursor comparison operator (">"/"<") to
// the equivalent MongoDB range operator ("$gt"/"$lt").
func mongoOperatorFor(operator string) string {
	if operator == ">" {
		return "$gt"
	}

	return "$lt"
}

// effectiveAscending reports whether the Mongo _id sort should be ascending.
// On the first page it follows the requested sort order (descending by default).
// On subsequent pages it follows the effective order computed from the cursor
// rules so that the _id range predicate and the sort scan agree.
func (r *MongoDBRepository) effectiveAscending(sortOrder string, isFirstPage bool, cursorDirection string) bool {
	if isFirstPage {
		return isAscending(sortOrder)
	}

	_, effectiveOrder, err := libHTTP.CursorDirectionRules(normalizeSortOrder(sortOrder), cursorDirection)
	if err != nil {
		return isAscending(sortOrder)
	}

	return effectiveOrder == "ASC"
}

// timestampRange returns the timestamp range sub-filter for the query, or nil
// when neither bound is set. Bounds are added only when non-zero.
func timestampRange(query AuditQuery) bson.M {
	if query.StartTime.IsZero() && query.EndTime.IsZero() {
		return nil
	}

	timeRange := bson.M{}

	if !query.StartTime.IsZero() {
		timeRange["$gte"] = query.StartTime
	}

	if !query.EndTime.IsZero() {
		timeRange["$lte"] = query.EndTime
	}

	return timeRange
}

// clampLimit normalizes the requested page size: non-positive falls back to the
// default, and values above the maximum are capped (max 100 per CLAUDE.md).
func clampLimit(limit int) int64 {
	if limit <= 0 {
		return defaultFindLimit
	}

	if limit > maxFindLimit {
		return maxFindLimit
	}

	return int64(limit)
}

// findOptions builds the Mongo find options: sort by _id (ascending when
// ascending is true, descending otherwise) and over-fetch one extra document
// beyond the clamped limit so the caller can detect a further page.
func findOptions(limit int64, ascending bool) *options.FindOptions {
	direction := -1
	if ascending {
		direction = 1
	}

	return options.Find().
		SetSort(bson.D{{Key: "_id", Value: direction}}).
		SetLimit(limit + 1)
}

// normalizeSortOrder maps the audit query sort order to an explicit "asc"/"desc"
// token before it is handed to libHTTP.CursorDirectionRules. The audit default
// is descending (newest-first); any value other than "asc" normalizes to "desc"
// so an empty/invalid order does not flip to ascending.
func normalizeSortOrder(sortOrder string) string {
	if isAscending(sortOrder) {
		return "asc"
	}

	return "desc"
}

// isAscending reports whether the sort order requests ascending (oldest-first)
// ordering. Any value other than "asc" defaults to descending (newest-first).
func isAscending(sortOrder string) bool {
	return sortOrder == "asc"
}

// getDatabase resolves the MongoDB database for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific *mongo.Database into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *MongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
	if r.connection == nil {
		if db := tmcore.GetMBContext(ctx); db != nil {
			return db, nil
		}

		return nil, fmt.Errorf("no database connection available: multi-tenant context required but not present, and no static connection configured")
	}

	if db := tmcore.GetMBContext(ctx); db != nil {
		return db, nil
	}

	return r.connection.Database(ctx)
}

// collection resolves the audit collection for the current request's database.
func (r *MongoDBRepository) collection(ctx context.Context) (*mongo.Collection, error) {
	db, err := r.getDatabase(ctx)
	if err != nil {
		return nil, err
	}

	return db.Collection(auditCollection), nil
}

// ensureIndexes ensures indexes exist for the audit collection.
// Uses per-database tracking to handle multi-tenant mode correctly.
// Retries on failure — indexes are only marked as done after successful creation.
func (r *MongoDBRepository) ensureIndexes(ctx context.Context, collection *mongo.Collection) error {
	key := collection.Database().Name() + ":" + auditCollection

	return globalIndexTracker.ensureOnce(key, func() error {
		return r.createIndexes(ctx, collection)
	})
}

// createIndexes builds the audit-log query and retention indexes.
//
// Query indexes are organization_id-led and _id-tailed: (organization_id, [filter], _id -1).
// Tenant isolation is physical (database-per-tenant via getDatabase/tmcore.GetMBContext),
// so tenant_id in the index would be redundant. Because _id is UUIDv7 (chronological),
// each (organization_id, [filter], _id -1) index serves the equality filter, the _id
// cursor range, and the _id sort in a single IXSCAN with no in-memory SORT. The
// optional timestamp range filter becomes an accepted residual predicate evaluated
// against the index-selected documents.
//
// request_id is sparse. expires_at is a TTL index honoring the per-document expiry
// written by the model; it is inert until a TTL emitter populates expires_at
// (SetExpireAfterSeconds(0) deletes once the per-document expires_at is reached).
func (r *MongoDBRepository) createIndexes(ctx context.Context, collection *mongo.Collection) error {
	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "organization_id", Value: 1}, {Key: "_id", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "organization_id", Value: 1}, {Key: "action", Value: 1}, {Key: "_id", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "organization_id", Value: 1}, {Key: "actor_id", Value: 1}, {Key: "_id", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "organization_id", Value: 1}, {Key: "outcome", Value: 1}, {Key: "_id", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "request_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexModels)

	return err
}

var _ Repository = (*MongoDBRepository)(nil)
