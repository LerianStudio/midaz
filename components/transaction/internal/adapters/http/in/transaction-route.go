package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
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

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)
	}

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

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)
	}

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

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)
	}

	return http.OK(c, transactionRoute)
}

// Delete a Transaction Route by ID.
//
//	@Summary		Delete Transaction Route by ID
//	@Description	Endpoint to delete a Transaction Route by its ID.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string								true	"Transaction Route ID in UUID format"
//	@Success		204				{object}	nil								"Successfully deleted transaction route"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Transaction Route not found"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [delete]
func (handler *TransactionRouteHandler) DeleteTransactionRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_transaction_route_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("transaction_route_id").(uuid.UUID)

	logger.Infof("Request to delete transaction route with ID: %s", id.String())

	err := handler.Command.DeleteTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete transaction route", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully deleted transaction route with ID: %s", id.String())

	if err := handler.Command.DeleteTransactionRouteCache(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete transaction route cache", err)

		logger.Errorf("Failed to delete transaction route cache: %v", err)
	}

	return http.NoContent(c)
}

// Get all Transaction Routes.
//
//	@Summary		Get all Transaction Routes
//	@Description	Endpoint to get all Transaction Routes with optional metadata filtering.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			limit			query		int									false	"Limit"			default(10)
//	@Param			start_date		query		string								false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string								false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string								false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string								false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.TransactionRoute,next_cursor=string,prev_cursor=string,limit=int,page=nil}
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes [get]
func (handler *TransactionRouteHandler) GetAllTransactionRoutes(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_transaction_routes")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:      headerParams.Limit,
		NextCursor: headerParams.Cursor,
		SortOrder:  headerParams.SortOrder,
		StartDate:  headerParams.StartDate,
		EndDate:    headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Transaction Routes by metadata")

		err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata headerParams to JSON string", err)

			return http.WithError(c, err)
		}

		transactionRoutes, cur, err := handler.Query.GetAllMetadataTransactionRoutes(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Transaction Routes by metadata", err)

			logger.Errorf("Failed to retrieve all Transaction Routes, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Transaction Routes by metadata")

		pagination.SetItems(transactionRoutes)
		pagination.SetCursor(cur.Next, cur.Prev)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Transaction Routes")

	headerParams.Metadata = &bson.M{}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	transactionRoutes, cur, err := handler.Query.GetAllTransactionRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Transaction Routes", err)

		logger.Errorf("Failed to retrieve all Transaction Routes, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Transaction Routes")

	pagination.SetItems(transactionRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}
