package in

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// SegmentHandler struct contains a segment use case for managing segment related operations.
type SegmentHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateSegment is a method that creates segment information.
//
//	@Summary		Create a Segment
//	@Description	Create a Segment with the input payload
//	@Tags			Segments
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			segment			body		mmodel.CreateSegmentInput	true	"Segment"
//	@Success		200				{object}	mmodel.Segment
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments [post]
func (handler *SegmentHandler) CreateSegment(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_segment")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Segment with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	payload := i.(*mmodel.CreateSegmentInput)
	logger.Infof("Request to create a Segment with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	segment, err := handler.Command.CreateSegment(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create Segment on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Segment")

	return http.Created(c, segment)
}

// GetAllSegments is a method that retrieves all Segments.
//
//	@Summary		Get all Segments
//	@Description	Get all Segments with the input metadata or without metadata
//	@Tags			Segments
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			metadata		query		string	false	"Metadata"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			page			query		int		false	"Page"			default(1)
//	@Param			start_date		query		string	false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Success		200				{object}	mpostgres.Pagination{items=[]mmodel.Segment,page=int,limit=int}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments [get]
func (handler *SegmentHandler) GetAllSegments(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_segments")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Get Segments with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := mpostgres.Pagination{
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
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Segments on query", err)

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
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Segments on query", err)

		logger.Errorf("Failed to retrieve all Segments, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Segments")

	pagination.SetItems(segments)

	return http.OK(c, pagination)
}

// GetSegmentByID is a method that retrieves Segment information by a given id.
//
//	@Summary		Get a Segment by ID
//	@Description	Get a Segment with the input ID
//	@Tags			Segments
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			id				path		string	true	"Segment ID"
//	@Success		200				{object}	mmodel.Segment
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id} [get]
func (handler *SegmentHandler) GetSegmentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_segment_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating retrieval of Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	segment, err := handler.Query.GetSegmentByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Segment on query", err)

		logger.Errorf("Failed to retrieve Segment with Ledger ID: %s and Segment ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.OK(c, segment)
}

// UpdateSegment is a method that updates Segment information.
//
//	@Summary		Update a Segment
//	@Description	Update a Segment with the input payload
//	@Tags			Segments
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			id				path		string						true	"Segment ID"
//	@Param			segment			body		mmodel.UpdateSegmentInput	true	"Segment"
//	@Success		200				{object}	mmodel.Segment
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id} [patch]
func (handler *SegmentHandler) UpdateSegment(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_segment")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	payload := i.(*mmodel.UpdateSegmentInput)
	logger.Infof("Request to update an Segment with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateSegmentByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Segment on command", err)

		logger.Errorf("Failed to update Segment with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	segment, err := handler.Query.GetSegmentByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Segment on query", err)

		logger.Errorf("Failed to retrieve Segment with Ledger ID: %s and Segment ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.OK(c, segment)
}

// DeleteSegmentByID is a method that removes Segment information by a given ids.
//
//	@Summary		Delete a Segment by ID
//	@Description	Delete a Segment with the input ID
//	@Tags			Segments
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path	string	true	"Organization ID"
//	@Param			ledger_id		path	string	true	"Ledger ID"
//	@Param			id				path	string	true	"Segment ID"
//	@Success		204
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id} [delete]
func (handler *SegmentHandler) DeleteSegmentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_segment_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	if err := handler.Command.DeleteSegmentByID(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove Segment on command", err)

		logger.Errorf("Failed to remove Segment with Ledger ID: %s and Segment ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Segment with Organization ID: %s and Ledger ID: %s and Segment ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.NoContent(c)
}
