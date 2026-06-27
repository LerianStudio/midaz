// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	// OperationHandler struct contains a cqrs use case for managing operations.
)

type OperationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// GetAllOperationsByAccount retrieves all operations by account.
//
//	@Summary		Get all Operations by account
//	@Description	Returns a cursor-paginated list of operations for an account. Operations are the individual debit/credit entries that make up transactions; filterable by date range, direction, and operation route.
//	@Tags			Operations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			account_id		path		string	true	"Account ID in UUID format"
//	@Param			limit			query		int		false	"Maximum number of items to return (max 100)"	default(10)
//	@Param			start_date		query		string	false	"Filter operations created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter operations created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort order by creation date"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Opaque cursor token for pagination"
//	@Param			type			query		string	false	"Filter by operation type"	Enums(DEBIT,CREDIT)
//	@Param			direction		query		string	false	"Filter by direction"	Enums(debit,credit)
//	@Param			route_id		query		string	false	"Filter by operation route ID"	format(uuid)
//	@Param			route_code		query		string	false	"Filter by operation route code"
//	@Success		200				{object}	http.Pagination{items=[]operation.Operation}	"Successfully retrieved operations list"
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations [get]
func (handler *OperationHandler) GetAllOperationsByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operations_by_account")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	accountID, err := http.GetUUIDFromLocals(c, "account_id")
	if err != nil {
		return http.WithError(c, err)
	}

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.WithError(c, err)
	}

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		recordSafeQueryAttributes(span, headerParams)

		trans, cur, err := handler.Query.GetAllMetadataOperations(ctx, organizationID, ledgerID, accountID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Operations by account and metadata", err)

			return http.WithError(c, err)
		}

		pagination.SetItems(trans)
		pagination.SetCursor(cur.Next, cur.Prev)

		return http.OK(c, pagination)
	}

	headerParams.Metadata = &bson.M{}

	operations, cur, err := handler.Query.GetAllOperationsByAccount(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Operations by account", err)

		return http.WithError(c, err)
	}

	pagination.SetItems(operations)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// GetOperationByAccount retrieves an operation by account.
//
//	@Summary		Get Operation
//	@Description	Get an Operation with the input ID
//	@Tags			Operations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			account_id		path		string	true	"Account ID in UUID format"
//	@Param			operation_id	path		string	true	"Operation ID in UUID format"
//	@Success		200				{object}	operation.Operation	"Successfully retrieved operation"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Operation not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations/{operation_id} [get]
func (handler *OperationHandler) GetOperationByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_by_account")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	accountID, err := http.GetUUIDFromLocals(c, "account_id")
	if err != nil {
		return http.WithError(c, err)
	}

	operationID, err := http.GetUUIDFromLocals(c, "operation_id")
	if err != nil {
		return http.WithError(c, err)
	}

	op, err := handler.Query.GetOperationByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Operation by account", err)

		return http.WithError(c, err)
	}

	return http.OK(c, op)
}

// UpdateOperation method that patch operation created before
//
//	@Summary		Update an Operation
//	@Description	Updates the mutable metadata of an operation. Amounts, accounts, direction, and type are immutable.
//	@Tags			Operations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			transaction_id	path		string							true	"Transaction ID in UUID format"
//	@Param			operation_id	path		string							true	"Operation ID in UUID format"
//	@Param			operation		body		operation.UpdateOperationInput	true	"Operation Input"
//	@Success		200				{object}	operation.Operation	"Successfully updated operation"
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Operation not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{operation_id} [patch]
func (handler *OperationHandler) UpdateOperation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_operation")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionID, err := http.GetUUIDFromLocals(c, "transaction_id")
	if err != nil {
		return http.WithError(c, err)
	}

	operationID, err := http.GetUUIDFromLocals(c, "operation_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*operation.UpdateOperationInput)

	recordSafePayloadAttributes(span, payload)

	logSafePayload(ctx, logger, "Request to update an Operation", payload)

	_, err = handler.Command.UpdateOperation(ctx, organizationID, ledgerID, transactionID, operationID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update Operation on command", err)

		return http.WithError(c, err)
	}

	op, err := handler.Query.GetOperationByID(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Operation on query", err)

		return http.WithError(c, err)
	}

	return http.OK(c, op)
}
