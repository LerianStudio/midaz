package in

import (
	"fmt"
	"os"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/attribute"
)

// LedgerHandler struct contains a ledger use case for managing ledger related operations.
type LedgerHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateLedger is a method that creates Ledger information.
//
//	@Summary		Create a new ledger
//	@Description	Creates a new ledger within the specified organization. A ledger is a financial record-keeping system for tracking assets, accounts, and transactions.
//	@Tags			Ledgers
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger			body		mmodel.CreateLedgerInput	true	"Ledger details including name, status, and optional metadata"
//	@Success		201				{object}	mmodel.Ledger				"Successfully created ledger"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Organization not found"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers [post]
func (handler *LedgerHandler) CreateLedger(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_ledger")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)

	payload := i.(*mmodel.CreateLedgerInput)
	logger.Infof("Request to create an ledger with details: %#v", payload)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	ledger, err := handler.Command.CreateLedger(ctx, organizationID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create ledger on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created ledger")

	return http.Created(c, ledger)
}

// GetLedgerByID is a method that retrieves Ledger information by a given id.
//
//	@Summary		Retrieve a specific ledger
//	@Description	Returns detailed information about a ledger identified by its UUID within the specified organization
//	@Tags			Ledgers
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			id				path		string	true	"Ledger ID in UUID format"
//	@Success		200				{object}	mmodel.Ledger	"Successfully retrieved ledger"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Ledger or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{id} [get]
func (handler *LedgerHandler) GetLedgerByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_ledger_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", id.String()),
	)

	logger.Infof("Initiating retrieval of Ledger with ID: %s", id.String())

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve ledger on query", err)

		logger.Errorf("Failed to retrieve Ledger with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Ledger with ID: %s", id.String())

	return http.OK(c, ledger)
}

// GetAllLedgers is a method that retrieves all ledgers.
//
//	@Summary		List all ledgers
//	@Description	Returns a paginated list of ledgers within the specified organization, optionally filtered by metadata, date range, and other criteria
//	@Tags			Ledgers
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			metadata		query		string	false	"JSON string to filter ledgers by metadata fields"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int		false	"Page number for pagination"									default(1)	minimum(1)
//	@Param			start_date		query		string	false	"Filter ledgers created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter ledgers created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Ledger,page=int,limit=int}	"Successfully retrieved ledgers list"
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers [get]
func (handler *LedgerHandler) GetAllLedgers(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_ledgers")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert query params to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Ledgers by metadata")

		ledgers, err := handler.Query.GetAllMetadataLedgers(ctx, organizationID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all ledgers by metadata", err)

			logger.Errorf("Failed to retrieve all Ledgers, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Ledgers by metadata")

		pagination.SetItems(ledgers)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Ledgers ")

	headerParams.Metadata = &bson.M{}

	ledgers, err := handler.Query.GetAllLedgers(ctx, organizationID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all ledgers on query", err)

		logger.Errorf("Failed to retrieve all Ledgers, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Ledgers")

	pagination.SetItems(ledgers)

	return http.OK(c, pagination)
}

// UpdateLedger is a method that updates Ledger information.
//
//	@Summary		Update an existing ledger
//	@Description	Updates a ledger's information such as name, status, or metadata. Only supplied fields will be updated.
//	@Tags			Ledgers
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			id				path		string						true	"Ledger ID in UUID format"
//	@Param			ledger			body		mmodel.UpdateLedgerInput	true	"Ledger fields to update. Only supplied fields will be modified."
//	@Success		200				{object}	mmodel.Ledger				"Successfully updated ledger"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Ledger or organization not found"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{id} [patch]
func (handler *LedgerHandler) UpdateLedger(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_ledger")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Ledger with ID: %s", id.String())

	organizationID := c.Locals("organization_id").(uuid.UUID)

	payload := p.(*mmodel.UpdateLedgerInput)
	logger.Infof("Request to update a Ledger with details: %#v", payload)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", id.String()),
	)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	_, err = handler.Command.UpdateLedgerByID(ctx, organizationID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update ledger on command", err)

		logger.Errorf("Failed to update Ledger with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve ledger on query", err)

		logger.Errorf("Failed to retrieve Ledger with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Ledger with ID: %s", id.String())

	return http.OK(c, ledger)
}

// DeleteLedgerByID is a method that removes Ledger information by a given id.
//
//	@Summary		Delete a ledger
//	@Description	Permanently removes a ledger identified by its UUID. Note: This operation is not available in production environments.
//	@Tags			Ledgers
//	@Param			Authorization	header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			id				path	string	true	"Ledger ID in UUID format"
//	@Success		204				{string}	string	"Ledger successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden action or not permitted in production environment"
//	@Failure		404				{object}	mmodel.Error	"Ledger or organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Cannot delete ledger with dependent resources"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{id} [delete]
func (handler *LedgerHandler) DeleteLedgerByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_ledger_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", id.String()),
	)

	logger.Infof("Initiating removal of Ledeger with ID: %s", id.String())

	if os.Getenv("ENV_NAME") == "production" {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove ledger on command", constant.ErrActionNotPermitted)

		logger.Errorf("Failed to remove Ledger with ID: %s in ", id.String())

		err := pkg.ValidateBusinessError(constant.ErrActionNotPermitted, reflect.TypeOf(mmodel.Ledger{}).Name())

		return http.WithError(c, err)
	}

	if err := handler.Command.DeleteLedgerByID(ctx, organizationID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove ledger on command", err)

		logger.Errorf("Failed to remove Ledeger with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Ledeger with ID: %s", id.String())

	return http.NoContent(c)
}

// CountLedgers is a method that returns the total count of ledgers for a specific organization.
//
//	@Summary		Count total ledgers
//	@Description	Returns the total count of ledgers for a specific organization as a header without a response body
//	@Tags			Ledgers
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Success		204				{string}	string	"No content with X-Total-Count header containing the count"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/metrics/count [head]
func (handler *LedgerHandler) CountLedgers(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_ledgers")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	logger.Infof("Initiating count of all ledgers for organization: %s", organizationID)

	count, err := handler.Query.CountLedgers(ctx, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count ledgers", err)
		logger.Errorf("Failed to count ledgers, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully counted ledgers for organization %s: %d", organizationID, count)

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
