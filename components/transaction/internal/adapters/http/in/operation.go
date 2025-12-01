package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// OperationHandler struct contains a cqrs use case for managing operations.
type OperationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// GetAllOperationsByAccount retrieves all operations by account.
//
//	@ID				listOperationsByAccount
//	@Summary		List all operations by account
//	@Description	Retrieves all operations associated with a specific account. Supports filtering by date range, operation type (DEBIT/CREDIT), cursor-based pagination, and sorting. Returns transaction details including amounts, balances before and after each operation.
//	@Tags			Operations
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			account_id		path		string	true	"Account ID in UUID format"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			start_date		query		string	false	"Filter records created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter records created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor for pagination to fetch the next set of results"
//	@Param			type			query		string	false	"Filter by operation type"									Enums(DEBIT,CREDIT)
//	@Success		200				{object}	libPostgres.Pagination{items=[]operation.Operation, next_cursor=string, prev_cursor=string,limit=int}	"Successfully retrieved operations"
//	@Example		response	{"items":[{"id":"op123456-89ab-cdef-0123-456789abcdef","transactionId":"t1234567-89ab-cdef-0123-456789abcdef","description":"Debit operation","type":"DEBIT","assetCode":"USD","chartOfAccounts":"1000","amount":{"value":"1500.00"},"balance":{"available":"16500.00","onHold":"500.00"},"balanceAfter":{"available":"15000.00","onHold":"500.00"},"status":{"code":"ACTIVE"},"accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","accountAlias":"@treasury","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"limit":10,"nextCursor":"eyJpZCI6Im9wMTIzNDU2In0="}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations [get]
func (handler *OperationHandler) GetAllOperationsByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operations_by_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Operations by account and metadata")

		err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata headerParams to JSON string", err)
		}

		trans, cur, err := handler.Query.GetAllMetadataOperations(ctx, organizationID, ledgerID, accountID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Operations by account and metadata", err)

			logger.Errorf("Failed to retrieve all Operations, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Operations by account and metadata")

		pagination.SetItems(trans)
		pagination.SetCursor(cur.Next, cur.Prev)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Operations by account")

	headerParams.Metadata = &bson.M{}

	operations, cur, err := handler.Query.GetAllOperationsByAccount(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Operations by account", err)

		logger.Errorf("Failed to retrieve all Operations by account, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Operations by account")

	pagination.SetItems(operations)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// GetOperationByAccount retrieves an operation by account.
//
//	@ID				getOperationByAccount
//	@Summary		Retrieve a specific operation
//	@Description	Returns detailed information about an operation identified by its UUID within the specified account, including amount, balance changes, and transaction reference
//	@Tags			Operations
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			account_id		path		string	true	"Account ID in UUID format"
//	@Param			operation_id	path		string	true	"Operation ID in UUID format"
//	@Success		200				{object}	operation.Operation	"Successfully retrieved operation"
//	@Example		response	{"id":"op123456-89ab-cdef-0123-456789abcdef","transactionId":"t1234567-89ab-cdef-0123-456789abcdef","description":"Debit operation","type":"DEBIT","assetCode":"USD","chartOfAccounts":"1000","amount":{"value":"1500.00"},"balance":{"available":"16500.00","onHold":"500.00"},"balanceAfter":{"available":"15000.00","onHold":"500.00"},"status":{"code":"ACTIVE"},"accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","accountAlias":"@treasury","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Operation not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations/{operation_id} [get]
func (handler *OperationHandler) GetOperationByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_by_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation by account")

	op, err := handler.Query.GetOperationByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Operation by account", err)

		logger.Errorf("Failed to retrieve Operation by account, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Operation by account")

	return http.OK(c, op)
}

// UpdateOperation method that patch operation created before
//
//	@ID				updateOperation
//	@Summary		Update an operation
//	@Description	Updates an existing operation's properties such as description and metadata. Only supplied fields will be updated.
//	@Tags			Operations
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			transaction_id	path		string							true	"Transaction ID in UUID format"
//	@Param			operation_id	path		string							true	"Operation ID in UUID format"
//	@Param			operation		body		operation.UpdateOperationInput	true	"Operation properties to update"
//	@Success		200				{object}	operation.Operation				"Successfully updated operation"
//	@Example		response	{"id":"op123456-89ab-cdef-0123-456789abcdef","transactionId":"t1234567-89ab-cdef-0123-456789abcdef","description":"Updated debit operation","type":"DEBIT","assetCode":"USD","chartOfAccounts":"1000","amount":{"value":"1500.00"},"balance":{"available":"16500.00","onHold":"500.00"},"balanceAfter":{"available":"15000.00","onHold":"500.00"},"status":{"code":"ACTIVE"},"accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","accountAlias":"@treasury","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T14:45:00Z"}
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Operation not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{operation_id} [patch]
func (handler *OperationHandler) UpdateOperation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_operation")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating update of Operation with Organization ID: %s, Ledger ID: %s, Transaction ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String(), operationID.String())

	payload := p.(*operation.UpdateOperationInput)

	if err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	logger.Infof("Request to update an Operation with details: %#v", payload)

	_, err := handler.Command.UpdateOperation(ctx, organizationID, ledgerID, transactionID, operationID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update Operation on command", err)

		logger.Errorf("Failed to update Operation with ID: %s, Error: %s", transactionID, err.Error())

		return http.WithError(c, err)
	}

	op, err := handler.Query.GetOperationByID(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Operation on query", err)

		logger.Errorf("Failed to retrieve Operation with ID: %s, Error: %s", operationID, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Operation with Organization ID: %s, Ledger ID: %s, Transaction ID: %s and ID: %s", organizationID, ledgerID, transactionID, operationID)

	return http.OK(c, op)
}
