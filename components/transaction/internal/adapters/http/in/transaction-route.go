package in

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/trace"
)

// TransactionRouteHandler handles HTTP requests for transaction route management.
// It provides CRUD operations for transaction routes within a ledger.
type TransactionRouteHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateTransactionRoute creates a new transaction route within a ledger.
//
//	@Summary		Create Transaction Route
//	@Description	Endpoint to create a new Transaction Route.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id		header		string								false	"Request ID for tracing"
//	@Param			organization_id		path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id			path		string								true	"Ledger ID in UUID format"
//	@Param			transaction-route	body		mmodel.CreateTransactionRouteInput	true	"Transaction Route Input"
//	@Success		201					{object}	mmodel.TransactionRoute				"Successfully created transaction route"
//	@Failure		400					{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error						"Forbidden access"
//	@Failure		500					{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes [post]
func (handler *TransactionRouteHandler) CreateTransactionRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_route")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")

	mlog.EnrichTransactionRoute(c, organizationID, ledgerID, uuid.Nil)
	mlog.SetHandler(c, "create_transaction_route")

	payload := http.Payload[*mmodel.CreateTransactionRouteInput](c, i)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	logger.Infof("Request to create a transaction route with details: %#v", payload)

	transactionRoute, err := handler.Command.CreateTransactionRoute(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route", err)

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully created transaction route")

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)
	}

	metricFactory.RecordTransactionRouteCreated(ctx, organizationID.String(), ledgerID.String())

	if err := http.Created(c, transactionRoute); err != nil {
		return err
	}

	return nil
}

// GetTransactionRouteByID retrieves a transaction route by its unique identifier.
//
//	@Summary		Get Transaction Route by ID
//	@Description	Endpoint to get a Transaction Route by its ID.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization			header		string					true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id			header		string					false	"Request ID for tracing"
//	@Param			organization_id			path		string					true	"Organization ID in UUID format"
//	@Param			ledger_id				path		string					true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string					true	"Transaction Route ID in UUID format"
//	@Success		200						{object}	mmodel.TransactionRoute	"Successfully retrieved transaction route"
//	@Failure		400						{object}	mmodel.Error			"Invalid input, validation errors"
//	@Failure		401						{object}	mmodel.Error			"Unauthorized access"
//	@Failure		403						{object}	mmodel.Error			"Forbidden access"
//	@Failure		404						{object}	mmodel.Error			"Transaction Route not found"
//	@Failure		500						{object}	mmodel.Error			"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [get]
func (handler *TransactionRouteHandler) GetTransactionRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction_route_by_id")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	id := http.LocalUUID(c, "transaction_route_id")

	mlog.EnrichTransactionRoute(c, organizationID, ledgerID, id)
	mlog.SetHandler(c, "get_transaction_route_by_id")

	logger.Infof("Request to get transaction route with ID: %s", id.String())

	transactionRoute, err := handler.Query.GetTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction route", err)

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully retrieved transaction route with ID: %s", id.String())

	if err := http.OK(c, transactionRoute); err != nil {
		return err
	}

	return nil
}

// UpdateTransactionRoute modifies an existing transaction route.
//
//	@Summary		Update Transaction Route
//	@Description	Endpoint to update a Transaction Route by its ID.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization			header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id			header		string								false	"Request ID for tracing"
//	@Param			organization_id			path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id				path		string								true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string								true	"Transaction Route ID in UUID format"
//	@Param			transaction-route		body		mmodel.UpdateTransactionRouteInput	true	"Transaction Route Input"
//	@Success		200						{object}	mmodel.TransactionRoute				"Successfully updated transaction route"
//	@Failure		400						{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401						{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403						{object}	mmodel.Error						"Forbidden access"
//	@Failure		500						{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [patch]
func (handler *TransactionRouteHandler) UpdateTransactionRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction_route")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	id := http.LocalUUID(c, "transaction_route_id")

	mlog.EnrichTransactionRoute(c, organizationID, ledgerID, id)
	mlog.SetHandler(c, "update_transaction_route")

	logger.Infof("Request to update transaction route with ID: %s", id.String())

	payload := http.Payload[*mmodel.UpdateTransactionRouteInput](c, i)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	logger.Infof("Request to update transaction route with details: %#v", payload)

	_, err = handler.Command.UpdateTransactionRoute(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction route", err)

		logger.Errorf("Failed to update transaction route with ID: %s, Error: %s", id.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	transactionRoute, err := handler.Query.GetTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction route", err)

		logger.Errorf("Failed to get transaction route with ID: %s, Error: %s", id.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully updated transaction route with ID: %s", id.String())

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)
	}

	if err := http.OK(c, transactionRoute); err != nil {
		return err
	}

	return nil
}

// DeleteTransactionRouteByID removes a transaction route by its unique identifier.
//
//	@Summary		Delete Transaction Route by ID
//	@Description	Endpoint to delete a Transaction Route by its ID.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization			header		string			true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id			header		string			false	"Request ID for tracing"
//	@Param			organization_id			path		string			true	"Organization ID in UUID format"
//	@Param			ledger_id				path		string			true	"Ledger ID in UUID format"
//	@Param			transaction_route_id	path		string			true	"Transaction Route ID in UUID format"
//	@Success		204						"Successfully deleted transaction route"
//	@Failure		400						{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401						{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403						{object}	mmodel.Error	"Forbidden access"
//	@Failure		404						{object}	mmodel.Error	"Transaction Route not found"
//	@Failure		500						{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id} [delete]
func (handler *TransactionRouteHandler) DeleteTransactionRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_transaction_route_by_id")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	id := http.LocalUUID(c, "transaction_route_id")

	mlog.EnrichTransactionRoute(c, organizationID, ledgerID, id)
	mlog.SetHandler(c, "delete_transaction_route_by_id")

	logger.Infof("Request to delete transaction route with ID: %s", id.String())

	err := handler.Command.DeleteTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete transaction route", err)

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully deleted transaction route with ID: %s", id.String())

	if err := handler.Command.DeleteTransactionRouteCache(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete transaction route cache", err)

		logger.Errorf("Failed to delete transaction route cache: %v", err)
	}

	if err := http.NoContent(c); err != nil {
		return err
	}

	return nil
}

// GetAllTransactionRoutes retrieves all transaction routes with optional filtering.
//
//	@Summary		Get all Transaction Routes
//	@Description	Endpoint to get all Transaction Routes with optional metadata filtering.
//	@Tags			Transaction Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example	"2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example	"2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.TransactionRoute,next_cursor=string,prev_cursor=string,limit=int,page=nil}
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes [get]
func (handler *TransactionRouteHandler) GetAllTransactionRoutes(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_transaction_routes")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")

	mlog.EnrichTransactionRoute(c, organizationID, ledgerID, uuid.Nil)
	mlog.SetHandler(c, "get_all_transaction_routes")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
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
		return handler.retrieveTransactionRoutesByMetadata(ctx, c, &span, logger, organizationID, ledgerID, headerParams, pagination)
	}

	return handler.retrieveAllTransactionRoutes(ctx, c, &span, logger, organizationID, ledgerID, headerParams, pagination)
}

// retrieveTransactionRoutesByMetadata retrieves transaction routes filtered by metadata.
func (handler *TransactionRouteHandler) retrieveTransactionRoutesByMetadata(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, headerParams *http.QueryHeader, pagination libPostgres.Pagination) error {
	logger.Infof("Initiating retrieval of all Transaction Routes by metadata")

	transactionRoutes, cur, err := handler.Query.GetAllMetadataTransactionRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Transaction Routes by metadata", err)

		logger.Errorf("Failed to retrieve all Transaction Routes, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully retrieved all Transaction Routes by metadata")

	pagination.SetItems(transactionRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return err
	}

	return nil
}

// retrieveAllTransactionRoutes retrieves all transaction routes without metadata filtering.
func (handler *TransactionRouteHandler) retrieveAllTransactionRoutes(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, headerParams *http.QueryHeader, pagination libPostgres.Pagination) error {
	logger.Infof("Initiating retrieval of all Transaction Routes")

	headerParams.Metadata = &bson.M{}

	err := libOpentelemetry.SetSpanAttributesFromStruct(span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert headerParams to JSON string", err)
	}

	transactionRoutes, cur, err := handler.Query.GetAllTransactionRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Transaction Routes", err)

		logger.Errorf("Failed to retrieve all Transaction Routes, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully retrieved all Transaction Routes")

	pagination.SetItems(transactionRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return err
	}

	return nil
}
