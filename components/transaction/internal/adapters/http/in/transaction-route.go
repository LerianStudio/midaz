package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
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
//	@ID				createTransactionRoute
//	@Summary		Create a new transaction route
//	@Description	Creates a new transaction route within the specified ledger. Transaction routes define how transactions are processed by linking operation routes that specify source and destination account matching rules.
//	@Tags			Transaction Route
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction-route	body		mmodel.CreateTransactionRouteInput	true	"Transaction route details including title, description, and operation route references"
//	@Success		201				{object}	mmodel.TransactionRoute				"Successfully created transaction route"
//	@Example		response	{"id":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","organizationId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","ledgerId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","title":"Wire Transfer Route","description":"Route for wire transfer operations","operationRoutes":[{"id":"d4e5f6a1-b2c3-7890-defa-4567890123ef","organizationId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","ledgerId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","title":"Source Account","code":"WIRE_SOURCE","operationType":"source","createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes [post]
func (handler *TransactionRouteHandler) CreateTransactionRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_route")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateTransactionRouteInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	logger.Infof("Request to create a transaction route with details: %#v", payload)

	transactionRoute, err := handler.Command.CreateTransactionRoute(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created transaction route")

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)
	}

	metricFactory.RecordTransactionRouteCreated(ctx, organizationID.String(), ledgerID.String())

	return http.Created(c, transactionRoute)
}

// Get a Transaction Route by ID.
//
//	@ID				getTransactionRouteByID
//	@Summary		Retrieve a specific transaction route
//	@Description	Returns detailed information about a transaction route identified by its UUID within the specified ledger, including all associated operation routes
//	@Tags			Transaction Route
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string								true	"Transaction Route ID in UUID format"
//	@Success		200				{object}	mmodel.TransactionRoute				"Successfully retrieved transaction route"
//	@Example		response	{"id":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","organizationId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","ledgerId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","title":"Wire Transfer Route","description":"Route for wire transfer operations","operationRoutes":[{"id":"d4e5f6a1-b2c3-7890-defa-4567890123ef","organizationId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","ledgerId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","title":"Source Account","code":"WIRE_SOURCE","operationType":"source","createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Transaction route not found"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [get]
func (handler *TransactionRouteHandler) GetTransactionRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction_route_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("transaction_route_id").(uuid.UUID)

	logger.Infof("Request to get transaction route with ID: %s", id.String())

	transactionRoute, err := handler.Query.GetTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction route", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved transaction route with ID: %s", id.String())

	return http.OK(c, transactionRoute)
}

// Update a Transaction Route.
//
//	@ID				updateTransactionRoute
//	@Summary		Update a transaction route
//	@Description	Updates an existing transaction route's properties such as title, description, and operation route associations. Only supplied fields will be updated.
//	@Tags			Transaction Route
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string								true	"Transaction Route ID in UUID format"
//	@Param			transaction-route	body		mmodel.UpdateTransactionRouteInput	true	"Transaction route properties to update"
//	@Success		200				{object}	mmodel.TransactionRoute				"Successfully updated transaction route"
//	@Example		response	{"id":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","organizationId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","ledgerId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","title":"Updated Wire Transfer Route","description":"Updated route for wire transfer operations","operationRoutes":[{"id":"d4e5f6a1-b2c3-7890-defa-4567890123ef","organizationId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","ledgerId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","title":"Source Account","code":"WIRE_SOURCE","operationType":"source","createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T14:45:00Z"}
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Transaction route not found"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [patch]
func (handler *TransactionRouteHandler) UpdateTransactionRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction_route")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("transaction_route_id").(uuid.UUID)

	logger.Infof("Request to update transaction route with ID: %s", id.String())

	payload := i.(*mmodel.UpdateTransactionRouteInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	logger.Infof("Request to update transaction route with details: %#v", payload)

	_, err = handler.Command.UpdateTransactionRoute(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction route", err)

		logger.Errorf("Failed to update transaction route with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	transactionRoute, err := handler.Query.GetTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction route", err)

		logger.Errorf("Failed to get transaction route with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated transaction route with ID: %s", id.String())

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)
	}

	return http.OK(c, transactionRoute)
}

// Delete a Transaction Route by ID.
//
//	@ID				deleteTransactionRoute
//	@Summary		Delete a transaction route
//	@Description	Permanently removes a transaction route identified by its UUID. This operation cannot be undone and will also remove associated cache entries.
//	@Tags			Transaction Route
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string								true	"Transaction Route ID in UUID format"
//	@Success		204				{object}	nil								"Successfully deleted transaction route"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Transaction route not found"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [delete]
func (handler *TransactionRouteHandler) DeleteTransactionRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_transaction_route_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("transaction_route_id").(uuid.UUID)

	logger.Infof("Request to delete transaction route with ID: %s", id.String())

	err := handler.Command.DeleteTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete transaction route", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully deleted transaction route with ID: %s", id.String())

	if err := handler.Command.DeleteTransactionRouteCache(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete transaction route cache", err)

		logger.Errorf("Failed to delete transaction route cache: %v", err)
	}

	return http.NoContent(c)
}

// Get all Transaction Routes.
//
//	@ID				listTransactionRoutes
//	@Summary		List all transaction routes
//	@Description	Retrieves all transaction routes within the specified ledger. Supports filtering by metadata, date range, cursor-based pagination, and sorting. Transaction routes define how transactions are processed and which operation routes to apply.
//	@Tags			Transaction Route
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			limit			query		int									false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			start_date		query		string								false	"Filter records created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string								false	"Filter records created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string								false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Param			cursor			query		string								false	"Cursor for pagination to fetch the next set of results"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.TransactionRoute,next_cursor=string,prev_cursor=string,limit=int,page=nil}	"Successfully retrieved transaction routes"
//	@Example		response	{"items":[{"id":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","organizationId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","ledgerId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","title":"Wire Transfer Route","description":"Route for wire transfer operations","operationRoutes":[{"id":"d4e5f6a1-b2c3-7890-defa-4567890123ef","organizationId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","ledgerId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","title":"Source Account","code":"WIRE_SOURCE","operationType":"source","createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"limit":10,"nextCursor":"eyJpZCI6InRyMTIzNDU2In0="}
//	@Failure		400				{object}	mmodel.Error						"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes [get]
func (handler *TransactionRouteHandler) GetAllTransactionRoutes(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_transaction_routes")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata headerParams to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Transaction Routes by metadata")

		transactionRoutes, cur, err := handler.Query.GetAllMetadataTransactionRoutes(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Transaction Routes by metadata", err)

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

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)
	}

	transactionRoutes, cur, err := handler.Query.GetAllTransactionRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Transaction Routes", err)

		logger.Errorf("Failed to retrieve all Transaction Routes, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Transaction Routes")

	pagination.SetItems(transactionRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}
