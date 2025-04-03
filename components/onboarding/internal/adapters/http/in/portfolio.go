package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// PortfolioHandler struct contains a portfolio use case for managing portfolio related operations.
type PortfolioHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreatePortfolio is a method that creates portfolio information.
//
//	@Summary		Create a new portfolio
//	@Description	Creates a new portfolio within the specified ledger. Portfolios represent collections of accounts grouped for specific purposes such as business units, departments, or client portfolios.
//	@Tags			Portfolios
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			portfolio		body		mmodel.CreatePortfolioInput	true	"Portfolio details including name, optional entity ID, status, and metadata"
//	@Success		201				{object}	mmodel.Portfolio			"Successfully created portfolio"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Organization or ledger not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Portfolio with the same name already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios [post]
func (handler *PortfolioHandler) CreatePortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_portfolio")

	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	logger.Infof("Initiating create of Portfolio with ledger ID: %s", ledgerID.String())

	payload := i.(*mmodel.CreatePortfolioInput)

	logger.Infof("Request to create a Portfolio with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	portfolio, err := handler.Command.CreatePortfolio(ctx, organizationID, ledgerID, payload)

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create Portfolio on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Portfolio")

	return http.Created(c, portfolio)
}

// GetAllPortfolios is a method that retrieves all Portfolios.
//
//	@Summary		List all portfolios
//	@Description	Returns a paginated list of portfolios within the specified ledger, optionally filtered by metadata, date range, and other criteria
//	@Tags			Portfolios
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			metadata		query		string	false	"JSON string to filter portfolios by metadata fields"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int		false	"Page number for pagination"									default(1)	minimum(1)
//	@Param			start_date		query		string	false	"Filter portfolios created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter portfolios created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Portfolio,page=int,limit=int}	"Successfully retrieved portfolios list"
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios [get]
func (handler *PortfolioHandler) GetAllPortfolios(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_portfolios")

	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Get Portfolios with Organization: %s and Ledger ID: %s", organizationID.String(), ledgerID.String())

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
		logger.Infof("Initiating retrieval of all Portfolios by metadata")

		portfolios, err := handler.Query.GetAllMetadataPortfolios(ctx, organizationID, ledgerID, *headerParams)

		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Portfolios on query", err)

			logger.Errorf("Failed to retrieve all Portfolios, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Portfolios by metadata")

		pagination.SetItems(portfolios)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Portfolios")

	headerParams.Metadata = &bson.M{}

	portfolios, err := handler.Query.GetAllPortfolio(ctx, organizationID, ledgerID, *headerParams)

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Portfolios on query", err)

		logger.Errorf("Failed to retrieve all Portfolios, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Portfolios")

	pagination.SetItems(portfolios)

	return http.OK(c, pagination)
}

// GetPortfolioByID is a method that retrieves Portfolio information by a given id.
//
//	@Summary		Retrieve a specific portfolio
//	@Description	Returns detailed information about a portfolio identified by its UUID within the specified ledger
//	@Tags			Portfolios
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			id				path		string	true	"Portfolio ID in UUID format"
//	@Success		200				{object}	mmodel.Portfolio	"Successfully retrieved portfolio"
//	@Failure		401				{object}	mmodel.Error		"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error		"Forbidden access"
//	@Failure		404				{object}	mmodel.Error		"Portfolio, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error		"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id} [get]
func (handler *PortfolioHandler) GetPortfolioByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_portfolio_by_id")

	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	portfolio, err := handler.Query.GetPortfolioByID(ctx, organizationID, ledgerID, id)

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Portfolio on query", err)

		logger.Errorf("Failed to retrieve Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return http.OK(c, portfolio)
}

// UpdatePortfolio is a method that updates Portfolio information.
//
//	@Summary		Update a portfolio
//	@Description	Updates an existing portfolio's properties such as name, entity ID, status, and metadata within the specified ledger
//	@Tags			Portfolios
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			id				path		string						true	"Portfolio ID in UUID format"
//	@Param			portfolio		body		mmodel.UpdatePortfolioInput	true	"Portfolio properties to update including name, entity ID, status, and optional metadata"
//	@Success		200				{object}	mmodel.Portfolio			"Successfully updated portfolio"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Portfolio, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Portfolio with the same name already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id} [patch]
func (handler *PortfolioHandler) UpdatePortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_portfolio")

	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	payload := i.(*mmodel.UpdatePortfolioInput)
	logger.Infof("Request to update an Portfolio with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdatePortfolioByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update Portfolio on command", err)

		logger.Errorf("Failed to update Portfolio with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	portfolio, err := handler.Query.GetPortfolioByID(ctx, organizationID, ledgerID, id)

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Portfolio on query", err)

		logger.Errorf("Failed to retrieve Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return http.OK(c, portfolio)
}

// DeletePortfolioByID is a method that removes Portfolio information by a given ids.
//
//	@Summary		Delete a portfolio
//	@Description	Permanently removes a portfolio from the specified ledger. This operation cannot be undone.
//	@Tags			Portfolios
//	@Param			Authorization	header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path	string	true	"Ledger ID in UUID format"
//	@Param			id				path	string	true	"Portfolio ID in UUID format"
//	@Success		204				{object}	nil	"Portfolio successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Portfolio, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Portfolio cannot be deleted due to existing dependencies"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id} [delete]
func (handler *PortfolioHandler) DeletePortfolioByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_portfolio_by_id")

	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	if err := handler.Command.DeletePortfolioByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove Portfolio on command", err)

		logger.Errorf("Failed to remove Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return http.NoContent(c)
}
