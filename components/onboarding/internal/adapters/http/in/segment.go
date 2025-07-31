package in

import (
	"strconv"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// SegmentHandler struct contains a segment use case for managing segment related operations.
type SegmentHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateSegment is a method that creates segment information.
//
//	@Summary		Create a new segment
//	@Description	Creates a new segment within the specified ledger. Segments represent logical divisions within a ledger, such as business areas, product lines, or customer categories.
//	@Tags			Segments
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			segment			body		mmodel.CreateSegmentInput	true	"Segment details including name, status, and optional metadata"
//	@Success		201				{object}	mmodel.Segment				"Successfully created segment"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Organization or ledger not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Segment with the same name already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments [post]
func (handler *SegmentHandler) CreateSegment(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_segment")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Segment with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	payload := i.(*mmodel.CreateSegmentInput)
	logger.Infof("Request to create a Segment with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	segment, err := handler.Command.CreateSegment(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create Segment on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Segment")

	return http.Created(c, segment)
}

// GetAllSegments is a method that retrieves all Segments.
//
//	@Summary		List all segments
//	@Description	Returns a paginated list of segments within the specified ledger, optionally filtered by metadata, date range, and other criteria
//	@Tags			Segments
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			metadata		query		string	false	"JSON string to filter segments by metadata fields"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int		false	"Page number for pagination"									default(1)	minimum(1)
//	@Param			start_date		query		string	false	"Filter segments created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter segments created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Segment,page=int,limit=int}	"Successfully retrieved segments list"
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments [get]
func (handler *SegmentHandler) GetAllSegments(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_segments")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Get Segments with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Segments by metadata")

		segments, err := handler.Query.GetAllMetadataSegments(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Segments on query", err)

			logger.Errorf("Failed to retrieve all Segments, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Segments by metadata")

		pagination.SetItems(segments)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Segments ")

	headerParams.Metadata = &bson.M{}

	segments, err := handler.Query.GetAllSegments(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Segments on query", err)

		logger.Errorf("Failed to retrieve all Segments, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Segments")

	pagination.SetItems(segments)

	return http.OK(c, pagination)
}

// GetSegmentByID is a method that retrieves Segment information by a given id.
//
//	@Summary		Retrieve a specific segment
//	@Description	Returns detailed information about a segment identified by its UUID within the specified ledger
//	@Tags			Segments
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			id				path		string	true	"Segment ID in UUID format"
//	@Success		200				{object}	mmodel.Segment	"Successfully retrieved segment"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Segment, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id} [get]
func (handler *SegmentHandler) GetSegmentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_segment_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating retrieval of Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	segment, err := handler.Query.GetSegmentByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Segment on query", err)

		logger.Errorf("Failed to retrieve Segment with Ledger ID: %s and Segment ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.OK(c, segment)
}

// UpdateSegment is a method that updates Segment information.
//
//	@Summary		Update a segment
//	@Description	Updates an existing segment's properties such as name, status, and metadata within the specified ledger
//	@Tags			Segments
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			id				path		string						true	"Segment ID in UUID format"
//	@Param			segment			body		mmodel.UpdateSegmentInput	true	"Segment properties to update including name, status, and optional metadata"
//	@Success		200				{object}	mmodel.Segment				"Successfully updated segment"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Segment, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Segment with the same name already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id} [patch]
func (handler *SegmentHandler) UpdateSegment(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_segment")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	payload := i.(*mmodel.UpdateSegmentInput)
	logger.Infof("Request to update an Segment with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateSegmentByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update Segment on command", err)

		logger.Errorf("Failed to update Segment with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	segment, err := handler.Query.GetSegmentByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Segment on query", err)

		logger.Errorf("Failed to retrieve Segment with Ledger ID: %s and Segment ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.OK(c, segment)
}

// DeleteSegmentByID is a method that removes Segment information by a given ids.
//
//	@Summary		Delete a segment
//	@Description	Permanently removes a segment from the specified ledger. This operation cannot be undone.
//	@Tags			Segments
//	@Param			Authorization	header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path	string	true	"Ledger ID in UUID format"
//	@Param			id				path	string	true	"Segment ID in UUID format"
//	@Success		204				{object}	nil	"Segment successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Segment, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Segment cannot be deleted due to existing dependencies"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id} [delete]
func (handler *SegmentHandler) DeleteSegmentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_segment_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	if err := handler.Command.DeleteSegmentByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove Segment on command", err)

		logger.Errorf("Failed to remove Segment with Ledger ID: %s and Segment ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.NoContent(c)
}

// CountSegments is a method that counts all segments for a given organization and ledger.
//
//	@Summary		Count segments
//	@Description	Returns the total count of segments for the specified organization and ledger
//	@Tags			Segments
//	@Param			Authorization	header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path	string	true	"Ledger ID in UUID format"
//	@Success		200				{object}	nil	"Successfully retrieved segments count"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/metrics/count [head]
func (handler *SegmentHandler) CountSegments(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_segments")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	logger.Infof("Counting segments for organization %s and ledger %s", organizationID, ledgerID)

	count, err := handler.Query.CountSegments(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count segments", err)
		logger.Errorf("Error counting segments: %v", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully counted segments for organization %s and ledger %s: %d", organizationID, ledgerID, count)

	c.Set(constant.XTotalCount, strconv.FormatInt(count, 10))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
