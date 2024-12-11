package in

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// PortfolioHandler struct contains a portfolio use case for managing portfolio related operations.
type PortfolioHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreatePortfolio is a method that creates portfolio information.
//
//	@Summary		Create a Portfolio
//	@Description	Create a Portfolio with the input payload
//	@Tags			Portfolios
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			portfolio		body		mmodel.CreatePortfolioInput	true	"Portfolio Payload"
//	@Success		200				{object}	mmodel.Portfolio
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios [post]
func (handler *PortfolioHandler) CreatePortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_portfolio")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	logger.Infof("Initiating create of Portfolio with ledger ID: %s", ledgerID.String())

	payload := i.(*mmodel.CreatePortfolioInput)

	logger.Infof("Request to create a Portfolio with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	portfolio, err := handler.Command.CreatePortfolio(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create Portfolio on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Portfolio")

	return http.Created(c, portfolio)
}

// GetAllPortfolios is a method that retrieves all Portfolios.
//
//	@Summary		Get all Portfolios
//	@Description	Get all Portfolios with the input metadata or without metadata
//	@Tags			Portfolios
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			metadata		query		string	false	"Metadata query"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			page			query		int		false	"Page"			default(1)
//	@Param			start_date		query		string	false	"Start Date"	example(2021-01-01)
//	@Param			end_date		query		string	false	"End Date"		example(2021-01-01)
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Success		200				{object}	mpostgres.Pagination{items=[]mmodel.Portfolio,page=int,limit=int}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios [get]
func (handler *PortfolioHandler) GetAllPortfolios(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_portfolios")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Get Portfolios with Organization: %s and Ledger ID: %s", organizationID.String(), ledgerID.String())

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
		logger.Infof("Initiating retrieval of all Portfolios by metadata")

		portfolios, err := handler.Query.GetAllMetadataPortfolios(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Portfolios on query", err)

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
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Portfolios on query", err)

		logger.Errorf("Failed to retrieve all Portfolios, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Portfolios")

	pagination.SetItems(portfolios)

	return http.OK(c, pagination)
}

// GetPortfolioByID is a method that retrieves Portfolio information by a given id.
//
//	@Summary		Get a Portfolio by ID
//	@Description	Get a Portfolio with the input ID
//	@Tags			Portfolios
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			id				path		string	true	"Portfolio ID"
//	@Success		200				{object}	mmodel.Portfolio
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id} [get]
func (handler *PortfolioHandler) GetPortfolioByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_portfolio_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	portfolio, err := handler.Query.GetPortfolioByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Portfolio on query", err)

		logger.Errorf("Failed to retrieve Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return http.OK(c, portfolio)
}

// UpdatePortfolio is a method that updates Portfolio information.
//
//	@Summary		Update a Portfolio
//	@Description	Update a Portfolio with the input payload
//	@Tags			Portfolios
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			id				path		string						true	"Portfolio ID"
//	@Param			portfolio		body		mmodel.UpdatePortfolioInput	true	"Portfolio Payload"
//	@Success		200				{object}	mmodel.Portfolio
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id} [patch]
func (handler *PortfolioHandler) UpdatePortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_portfolio")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	payload := i.(*mmodel.UpdatePortfolioInput)
	logger.Infof("Request to update an Portfolio with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdatePortfolioByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Portfolio on command", err)

		logger.Errorf("Failed to update Portfolio with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	portfolio, err := handler.Query.GetPortfolioByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Portfolio on query", err)

		logger.Errorf("Failed to retrieve Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return http.OK(c, portfolio)
}

// DeletePortfolioByID is a method that removes Portfolio information by a given ids.
//
//	@Summary		Delete a Portfolio by ID
//	@Description	Delete a Portfolio with the input ID
//	@Tags			Portfolios
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path	string	true	"Organization ID"
//	@Param			ledger_id		path	string	true	"Ledger ID"
//	@Param			id				path	string	true	"Portfolio ID"
//	@Success		204
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id} [delete]
func (handler *PortfolioHandler) DeletePortfolioByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_portfolio_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	if err := handler.Command.DeletePortfolioByID(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove Portfolio on command", err)

		logger.Errorf("Failed to remove Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return http.NoContent(c)
}
