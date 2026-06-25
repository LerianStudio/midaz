// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// AuditHandler handles HTTP requests for protection audit event queries.
type AuditHandler struct {
	Service encryption.AuditQueryService
}

// auditEventResponse is a single audit event in the API response.
//
// It deliberately excludes internal-only fields (EventType, TenantID,
// ActorType, and the AffectedKeyIDs/ProviderReference/ErrorCode details),
// lifting only the previous/new status out of Details.
type auditEventResponse struct {
	ID         string `json:"id"`
	Action     string `json:"action"`
	Actor      string `json:"actor"`
	Outcome    string `json:"outcome"`
	Reason     string `json:"reason"`
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	Timestamp  string `json:"timestamp"`
	RequestID  string `json:"request_id"`
}

// auditEventsEnvelope is the top-level response for the audit events listing.
//
// It mirrors the cursor-pagination keys of http.Pagination (items, limit,
// next_cursor, prev_cursor) but adds the top-level organization_id, so it is a
// dedicated local envelope rather than a reuse of http.Pagination.
type auditEventsEnvelope struct {
	OrganizationID string               `json:"organization_id"`
	Items          []auditEventResponse `json:"items"`
	Limit          int                  `json:"limit"`
	NextCursor     string               `json:"next_cursor,omitempty"`
	PrevCursor     string               `json:"prev_cursor,omitempty"`
}

// allowedAuditOutcomes is the reduced Phase-1 outcome enum accepted as a filter.
// conflict and not_found are deferred and intentionally rejected.
var allowedAuditOutcomes = map[string]struct{}{
	string(mmodel.AuditOutcomeSuccess):       {},
	string(mmodel.AuditOutcomeFailure):       {},
	string(mmodel.AuditOutcomeAlreadyExists): {},
}

// GetAuditEvents handles the retrieval of protection audit events for an organization.
//
//	@Summary		List Protection Audit Events
//	@Description	Returns the protection audit events for an organization, filtered by action, actor, outcome, and time range, with cursor pagination.
//	@Tags			Protection
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path		string	true	"The unique identifier of the Organization."
//	@Param			limit				query		int		false	"Maximum number of events to return (default 20)."
//	@Param			cursor				query		string	false	"Opaque pagination cursor."
//	@Param			sort_order			query		string	false	"Sort order: asc or desc (default desc)."
//	@Param			action				query		string	false	"Filter by action."
//	@Param			actor				query		string	false	"Filter by actor."
//	@Param			outcome				query		string	false	"Filter by outcome: success, failure, or already_exists."
//	@Param			start_date			query		string	false	"Inclusive lower time bound (yyyy-mm-dd or RFC3339)."
//	@Param			end_date			query		string	false	"Inclusive upper time bound (yyyy-mm-dd or RFC3339)."
//	@Success		200					{object}	in.auditEventsEnvelope
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/organizations/{organization_id}/protection/audit [get]
func (handler *AuditHandler) GetAuditEvents(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_audit_events")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	queries := c.Queries()

	// Filter start_date/end_date out of the map handed to ValidateParameters so
	// it validates ONLY limit/cursor/sort_order and never touches dates. This
	// makes parseAuditTime the sole date authority below, giving per-bound,
	// unbounded-on-absent semantics: ValidateParameters.validateDates would
	// otherwise inject a default range when both are absent, reject single-sided
	// bounds, and enforce a max-range window — none of which this endpoint wants.
	// The original `queries` is left intact for the limit/sort_order presence
	// checks and for parseAuditTime below.
	validateQueries := make(map[string]string, len(queries))
	for key, value := range queries {
		if key == "start_date" || key == "end_date" {
			continue
		}

		validateQueries[key] = value
	}

	headerParams, err := http.ValidateParameters(validateQueries)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate query parameters", libLog.Err(err))

		return http.WithError(c, err)
	}

	// ValidateParameters applies generic defaults (limit 10, sort_order asc) that
	// only kick in for present-but-empty values; an ABSENT key reaches here with
	// those generic defaults already set. This endpoint requires limit 20 / desc
	// when the key is absent, so override based on raw key presence. A
	// present-but-invalid value still flows through ValidateParameters.
	if _, ok := queries["limit"]; !ok {
		headerParams.Limit = 20
	}

	if _, ok := queries["sort_order"]; !ok {
		headerParams.SortOrder = "desc"
	}

	action := queries["action"]
	actor := queries["actor"]
	outcome := queries["outcome"]

	if outcome != "" {
		if _, ok := allowedAuditOutcomes[outcome]; !ok {
			err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "outcome")

			logger.Log(ctx, libLog.LevelWarn, "Rejected unsupported audit outcome filter", libLog.Err(err))

			return http.WithError(c, err)
		}
	}

	startTime, err := parseAuditTime(queries["start_date"], false)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "start_date")

		logger.Log(ctx, libLog.LevelWarn, "Rejected unparseable start_date", libLog.Err(err))

		return http.WithError(c, err)
	}

	endTime, err := parseAuditTime(queries["end_date"], true)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "end_date")

		logger.Log(ctx, libLog.LevelWarn, "Rejected unparseable end_date", libLog.Err(err))

		return http.WithError(c, err)
	}

	// ValidateParameters' date validation is intentionally bypassed for this
	// endpoint, so the inverted-range rejection it would provide is reapplied
	// here once both bounds are known.
	if !startTime.IsZero() && !endTime.IsZero() && startTime.After(endTime) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "start_date")

		logger.Log(ctx, libLog.LevelWarn, "Rejected inverted audit date range", libLog.Err(err))

		return http.WithError(c, err)
	}

	// Safe attributes only: org id, paging shape, and which filters are set —
	// never the filter VALUES (action/actor/outcome) or time bounds.
	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.Int("app.request.limit", headerParams.Limit),
		attribute.String("app.request.sort_order", headerParams.SortOrder),
		attribute.Bool("app.request.filter_action", action != ""),
		attribute.Bool("app.request.filter_actor", actor != ""),
		attribute.Bool("app.request.filter_outcome", outcome != ""),
		attribute.Bool("app.request.filter_start_time", !startTime.IsZero()),
		attribute.Bool("app.request.filter_end_time", !endTime.IsZero()),
	)

	query := audit.AuditQuery{
		Limit:     headerParams.Limit,
		Cursor:    headerParams.Cursor,
		SortOrder: headerParams.SortOrder,
		Action:    action,
		Actor:     actor,
		Outcome:   outcome,
		StartTime: startTime,
		EndTime:   endTime,
	}

	events, pagination, err := handler.Service.GetAuditEvents(ctx, organizationID.String(), query)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get audit events", err)

		if errors.Is(err, libHTTP.ErrInvalidCursor) {
			logger.Log(ctx, libLog.LevelWarn, "Rejected invalid pagination cursor", libLog.Err(err))

			return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "cursor"))
		}

		logger.Log(ctx, libLog.LevelError, "Failed to get audit events", libLog.Err(err))

		return http.WithError(c, err)
	}

	items := make([]auditEventResponse, 0, len(events))
	for _, event := range events {
		items = append(items, toAuditEventResponse(event))
	}

	envelope := auditEventsEnvelope{
		OrganizationID: organizationID.String(),
		Items:          items,
		Limit:          headerParams.Limit,
		NextCursor:     pagination.Next,
		PrevCursor:     pagination.Prev,
	}

	return http.OK(c, envelope)
}

// parseAuditTime is the SOLE date validator for this endpoint. ValidateParameters
// is intentionally bypassed for start_date/end_date (the keys are filtered out of
// its input in GetAuditEvents) because its validateDates injects a default range
// when both bounds are absent, rejects single-sided bounds, and enforces a
// max-range window — semantics this endpoint does not want. parseAuditTime instead
// treats each bound independently and unbounded-on-absent.
//
// It returns a zero time for an absent (empty) value so the repository treats the
// bound as unset. A present value is parsed with libCommons.ParseDateTime — the
// same parser the rest of CRM uses — so the accepted format set (yyyy-mm-dd,
// RFC3339, yyyy-mm-dd hh:mm:ss) is consistent across the endpoint. isEndDate
// normalizes a date-only end bound to end-of-day. A present-but-unparseable value
// yields an error, surfaced as a 400 by the caller.
func parseAuditTime(value string, isEndDate bool) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	parsed, _, err := libCommons.ParseDateTime(value, isEndDate)
	if err != nil {
		return time.Time{}, err
	}

	return parsed, nil
}

// toAuditEventResponse maps a domain audit event to its API representation,
// lifting the previous/new status out of Details and excluding internal-only
// fields. A nil Details yields empty status strings.
func toAuditEventResponse(event *mmodel.ProtectionAuditEvent) auditEventResponse {
	var fromStatus, toStatus string
	if event.Details != nil {
		fromStatus = event.Details.PreviousStatus
		toStatus = event.Details.NewStatus
	}

	return auditEventResponse{
		ID:         event.ID.String(),
		Action:     string(event.Action),
		Actor:      event.ActorID,
		Outcome:    string(event.Outcome),
		Reason:     event.Reason,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
		Timestamp:  event.Timestamp.UTC().Format(time.RFC3339),
		RequestID:  event.RequestID,
	}
}
