package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type TransactionRouteHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// Create a Transaction Route.
//
//	@Summary		Create Transaction Route
//	@Description	Endpoint to create a new Transaction Route.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction-route	body		mmodel.CreateTransactionRouteInput	true	"Transaction Route Input"
//	@Success		201				{object}	mmodel.TransactionRoute				"Successfully created transaction route"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes [post]
func (handler *TransactionRouteHandler) CreateTransactionRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_route")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateTransactionRouteInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	logger.Infof("Request to create a transaction route with details: %#v", payload)

	transactionRoute, err := handler.Command.CreateTransactionRoute(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created transaction route")

	return http.Created(c, transactionRoute)
}

// Get a Transaction Route by ID.
//
//	@Summary		Get Transaction Route by ID
//	@Description	Endpoint to get a Transaction Route by its ID.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string								true	"Transaction Route ID in UUID format"
//	@Success		200				{object}	mmodel.TransactionRoute				"Successfully retrieved transaction route"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Transaction Route not found"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [get]
func (handler *TransactionRouteHandler) GetTransactionRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction_route_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("transaction_route_id").(uuid.UUID)

	logger.Infof("Request to get transaction route with ID: %s", id.String())

	transactionRoute, err := handler.Query.GetTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction route", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved transaction route with ID: %s", id.String())

	return http.OK(c, transactionRoute)
}

// Update a Transaction Route.
//
//	@Summary		Update Transaction Route
//	@Description	Endpoint to update a Transaction Route by its ID.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string								true	"Transaction Route ID in UUID format"
//	@Param			transaction-route	body		mmodel.UpdateTransactionRouteInput	true	"Transaction Route Input"
//	@Success		200				{object}	mmodel.TransactionRoute				"Successfully updated transaction route"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [patch]
func (handler *TransactionRouteHandler) UpdateTransactionRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction_route")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("transaction_route_id").(uuid.UUID)

	logger.Infof("Request to update transaction route with ID: %s", id.String())

	payload := i.(*mmodel.UpdateTransactionRouteInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	logger.Infof("Request to update transaction route with details: %#v", payload)

	_, err = handler.Command.UpdateTransactionRoute(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update transaction route", err)

		logger.Errorf("Failed to update transaction route with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	transactionRoute, err := handler.Query.GetTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction route", err)

		logger.Errorf("Failed to get transaction route with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated transaction route with ID: %s", id.String())

	return http.OK(c, transactionRoute)
}
