// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
)

type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}
<<<<<<< HEAD
=======

// CreateTransactionJSON method that create transaction using JSON
//
//	@Summary		Create a Transaction using JSON
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string										true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string										false	"Request ID"
//	@Param			organization_id	path		string										true	"Organization ID"
//	@Param			ledger_id		path		string										true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json [post]
func (handler *TransactionHandler) CreateTransactionJSON(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*transaction.CreateTransactionInput)
	transactionInput := input.BuildTransaction()
	logger.Infof("Request to create an transaction with details: %#v", transactionInput)

	transactionStatus := constant.CREATED
	if transactionInput.Pending {
		transactionStatus = constant.PENDING
	}

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", transactionInput)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	return handler.createTransaction(c, *transactionInput, transactionStatus)
}

// CreateTransactionAnnotation method that create transaction using JSON
//
//	@Summary		Create a Transaction Annotation using JSON
//	@Description	Create a Transaction Annotation with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string										true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string										false	"Request ID"
//	@Param			organization_id	path		string										true	"Organization ID"
//	@Param			ledger_id		path		string										true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/annotation [post]
func (handler *TransactionHandler) CreateTransactionAnnotation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*transaction.CreateTransactionInput)
	transactionInput := input.BuildTransaction()
	logger.Infof("Create an transaction annotation without an affected balance: %#v", transactionInput)

	return handler.createTransaction(c, *transactionInput, constant.NOTED)
}

// CreateTransactionInflow method that creates a transaction without specifying a source
//
//	@Summary		Create a Transaction without passing from source
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string											true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string											false	"Request ID"
//	@Param			organization_id	path		string											true	"Organization ID"
//	@Param			ledger_id		path		string											true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionInflowSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/inflow [post]
func (handler *TransactionHandler) CreateTransactionInflow(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_inflow")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*transaction.CreateTransactionInflowInput)
	transactionInput := input.BuildInflowEntry()
	logger.Infof("Request to create an transaction inflow with details: %#v", transactionInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", transactionInput)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	response := handler.createTransaction(c, *transactionInput, constant.CREATED)

	return response
}

// CreateTransactionOutflow method that creates a transaction without specifying a distribution
//
//	@Summary		Create a Transaction without passing to distribution
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string												true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string												false	"Request ID"
//	@Param			organization_id	path		string												true	"Organization ID"
//	@Param			ledger_id		path		string												true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionOutflowSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/outflow [post]
func (handler *TransactionHandler) CreateTransactionOutflow(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_outflow")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*transaction.CreateTransactionOutflowInput)
	transactionInput := input.BuildOutflowEntry()
	logger.Infof("Request to create an transaction outflow with details: %#v", transactionInput)

	transactionStatus := constant.CREATED
	if transactionInput.Pending {
		transactionStatus = constant.PENDING
	}

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", transactionInput)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	return handler.createTransaction(c, *transactionInput, transactionStatus)
}

// CreateTransactionDSL method that create transaction using DSL
//
//	@Summary		Create a Transaction using DSL
//	@Description	Create a Transaction with the input DSL file
//	@Tags			Transactions
//	@Accept			mpfd
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction		formData	file	true	"Transaction DSL file"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid DSL file format or validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl [post]
func (handler *TransactionHandler) CreateTransactionDSL(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_dsl")
	defer span.End()

	c.SetUserContext(ctx)

	// RFC 8594 Deprecation Headers
	// Indicates this endpoint is deprecated and will be removed on the sunset date.
	// Clients should migrate to POST /transactions/json endpoint.
	c.Set("Deprecation", "true")
	c.Set("Sunset", "Sat, 01 Aug 2026 00:00:00 GMT")
	c.Set("Link", "</v1/organizations/"+c.Params("organization_id")+
		"/ledgers/"+c.Params("ledger_id")+
		"/transactions/json>; rel=\"successor-version\"")

	logger.Warnf("DEPRECATED ENDPOINT: POST /transactions/dsl called. "+
		"request_id=%s. Use POST /transactions/json instead. "+
		"Sunset date: 2026-08-01", c.Get("X-Request-Id"))

	_, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Error("Failed to validate query parameters: ", err.Error())

		return http.WithError(c, err)
	}

	dsl, err := http.GetFileFromHeader(c)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get file from Header", err)

		logger.Error("Failed to get file from Header: ", err.Error())

		return http.WithError(c, err)
	}

	errListener := goldTransaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name(), errListener.Errors)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate script in DSL", err)

		return http.WithError(c, err)
	}

	parsed := goldTransaction.Parse(dsl)

	transactionInput, ok := parsed.(pkgTransaction.Transaction)
	if !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to parse script in DSL", err)

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.transaction.send", transactionInput.Send)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	response := handler.createTransaction(c, transactionInput, constant.CREATED)

	return response
}

// CommitTransaction method that commit transaction created before
//
//	@Summary		Commit a Transaction
//	@Description	Commit a previously created transaction
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit [Post]
func (handler *TransactionHandler) CommitTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	_, span := tracer.Start(ctx, "handler.commit_transaction")
	defer span.End()

	tran, err := handler.Query.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		tran, err = handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

			logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

			return http.WithError(c, err)
		}
	}

	return handler.commitOrCancelTransaction(c, tran, constant.APPROVED)
}

// CancelTransaction method that cancel pre transaction created before
//
//	@Summary		Cancel a pre transaction
//	@Description	Cancel a previously created pre transaction
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel [Post]
func (handler *TransactionHandler) CancelTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	_, span := tracer.Start(ctx, "handler.cancel_transaction")
	defer span.End()

	tran, err := handler.Query.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		tran, err = handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

			logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

			return http.WithError(c, err)
		}
	}

	return handler.commitOrCancelTransaction(c, tran, constant.CANCELED)
}

// RevertTransaction method that revert transaction created before
//
//	@Summary		Revert a Transaction
//	@Description	Revert a Transaction with Transaction ID only
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert [post]
func (handler *TransactionHandler) RevertTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	parent, err := handler.Query.GetParentByTransactionID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Parent Transaction on query", err)

		logger.Errorf("Failed to retrieve Parent Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	if parent != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction Has Already Parent Transaction", err)

		logger.Errorf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return http.WithError(c, err)
	}

	tran, err := handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	if tran.ParentTransactionID != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction Has Already Parent Transaction", err)

		logger.Errorf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return http.WithError(c, err)
	}

	if tran.Status.Code != constant.APPROVED {
		err = pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction CantRevert Transaction", err)

		logger.Errorf("Transaction CantRevert Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return http.WithError(c, err)
	}

	transactionReverted := tran.TransactionRevert()
	if transactionReverted.IsEmpty() {
		err = pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction can't be reverted", err)

		logger.Errorf("Parent Transaction can't be reverted with ID: %s, Error: %s", transactionID.String(), err)

		return http.WithError(c, err)
	}

	// Validate bidirectional routes: operations with a route_id require
	// the referenced OperationRoute to have OperationType "bidirectional".
	// Collect unique route IDs first to avoid N+1 queries.
	uniqueRouteIDs := make(map[uuid.UUID]struct{})
	for _, op := range tran.Operations {
		if op.Route == "" {
			continue
		}

		routeUUID, parseErr := uuid.Parse(op.Route)
		if parseErr != nil {
			continue
		}

		uniqueRouteIDs[routeUUID] = struct{}{}
	}

	for routeUUID := range uniqueRouteIDs {
		operationRoute, routeErr := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, routeUUID)
		if routeErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve operation route for revert validation", routeErr)

			logger.Errorf("Failed to retrieve operation route %s for revert validation: %v", routeUUID.String(), routeErr)

			return http.WithError(c, routeErr)
		}

		if operationRoute != nil && operationRoute.OperationType != "bidirectional" {
			err = pkg.ValidateBusinessError(constant.ErrRouteNotBidirectional, "RevertTransaction")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation route is not bidirectional", err)

			logger.Errorf("Operation route %s is not bidirectional (type: %s), cannot revert transaction %s",
				routeUUID.String(), operationRoute.OperationType, transactionID.String())

			return http.WithError(c, err)
		}
	}

	response := handler.createTransaction(c, transactionReverted, constant.CREATED)

	return response
}

// UpdateTransaction method that patch transaction created before
//
//	@Summary		Update a Transaction
//	@Description	Update a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction_id	path		string								true	"Transaction ID"
//	@Param			transaction		body		transaction.UpdateTransactionInput	true	"Transaction Input"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [patch]
func (handler *TransactionHandler) UpdateTransaction(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	logger.Infof("Initiating update of Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String())

	payload := p.(*transaction.UpdateTransactionInput)
	logger.Infof("Request to update an Transaction with details: %#v", payload)

	if err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	_, err := handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction on command", err)

		logger.Errorf("Failed to update Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String())

	return http.OK(c, trans)
}

// GetTransaction method that get transaction created before
//
//	@Summary		Get a Transaction by ID
//	@Description	Get a Transaction with the input ID
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [get]
func (handler *TransactionHandler) GetTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	// Try to get transaction from write-behind cache (operations already embedded)
	if wbTran, wbErr := handler.Query.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID); wbErr == nil {
		c.Set("X-Cache-Hit", "true")

		return http.OK(c, wbTran)
	}

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	ctxGetTransaction, spanGetTransaction := tracer.Start(ctx, "handler.get_transaction.get_operations")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	headerParams.Metadata = &bson.M{}

	tran, err = handler.Query.GetOperationsByTransaction(ctxGetTransaction, organizationID, ledgerID, tran, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanGetTransaction, "Failed to retrieve Operations", err)

		logger.Errorf("Failed to retrieve Operations with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	spanGetTransaction.End()

	logger.Infof("Successfully retrieved Transaction with ID: %s", transactionID.String())

	return http.OK(c, tran)
}

// GetAllTransactions method that get all transactions created before
//
//	@Summary		Get all Transactions
//	@Description	Get all Transactions with the input metadata or without metadata
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example	"2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example	"2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]transaction.Transaction,next_cursor=string,prev_cursor=string,limit=int,page=nil}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions [get]
func (handler *TransactionHandler) GetAllTransactions(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_transactions")
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
		logger.Infof("Initiating retrieval of all Transactions by metadata")

		trans, cur, err := handler.Query.GetAllMetadataTransactions(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Transactions by metadata", err)

			logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Transactions by metadata")

		pagination.SetItems(trans)
		pagination.SetCursor(cur.Next, cur.Prev)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Transactions ")

	headerParams.Metadata = &bson.M{}

	trans, cur, err := handler.Query.GetAllTransactions(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Transactions", err)

		logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Transactions")

	pagination.SetItems(trans)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// HandleAccountFields processes account and accountAlias fields for transaction entries
// accountAlias is deprecated but still needs to be handled for backward compatibility
func (handler *TransactionHandler) HandleAccountFields(entries []pkgTransaction.FromTo, isConcat bool) []pkgTransaction.FromTo {
	result := make([]pkgTransaction.FromTo, 0, len(entries))

	for i := range entries {
		var newAlias string
		if isConcat {
			newAlias = entries[i].ConcatAlias(i)
		} else {
			newAlias = entries[i].SplitAlias()
		}

		entries[i].AccountAlias = newAlias

		result = append(result, entries[i])
	}

	return result
}

func (handler *TransactionHandler) checkTransactionDate(logger libLog.Logger, transactionInput pkgTransaction.Transaction, transactionStatus string) (time.Time, error) {
	now := time.Now()
	transactionDate := now

	if transactionInput.TransactionDate != nil && !transactionInput.TransactionDate.IsZero() {
		if transactionInput.TransactionDate.After(now) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidFutureTransactionDate, "validateTransactionDate")

			logger.Warnf("transaction date cannot be a future date: %v", err.Error())

			return time.Time{}, err
		} else if transactionStatus == constant.PENDING {
			err := pkg.ValidateBusinessError(constant.ErrInvalidPendingFutureTransactionDate, "validateTransactionDate")

			logger.Warnf("pending transaction cannot be used together a transaction date: %v", err.Error())

			return time.Time{}, err
		} else {
			transactionDate = transactionInput.TransactionDate.Time()
		}
	}

	return transactionDate, nil
}

// BuildOperations builds the operations for the transaction.
// When route validation is enabled (via LedgerSettings) and the transaction is PENDING,
// source entries generate 2 operations (DEBIT debit + ONHOLD credit) instead of 1 ONHOLD,
// implementing proper double-entry accounting where each operation affects a single balance field.
func (handler *TransactionHandler) BuildOperations(
	ctx context.Context,
	balances []*mmodel.Balance,
	fromTo []pkgTransaction.FromTo,
	transactionInput pkgTransaction.Transaction,
	tran transaction.Transaction,
	validate *pkgTransaction.Responses,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*operation.Operation, []*mmodel.Balance, error) {
	var operations []*operation.Operation

	var preBalances []*mmodel.Balance

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction_operations")
	defer span.End()

	orgID, _ := uuid.Parse(tran.OrganizationID)
	ledID, _ := uuid.Parse(tran.LedgerID)

	ledgerSettings := handler.Query.GetLedgerSettings(ctx, orgID, ledID)

	// Callers (createTransaction, commitOrCancelTransaction) must call propagateRouteValidation
	// before invoking BuildOperations so that Amount entries already carry the flag.
	routeValidationEnabled := ledgerSettings.Accounting.ValidateRoutes

	if routeValidationEnabled {
		logger.Infof("Route validation enabled for ledger %s, applying double-entry operations", tran.LedgerID)

		span.SetAttributes(attribute.Bool("app.route_validation_enabled", true))
	}

	// Track aliases that already had double-entry operations built, so we skip
	// the second Lua before-balance for the same source account.
	processedDoubleEntry := make(map[string]bool)

	for _, blc := range balances {
		for i := range fromTo {
			if blc.Alias == fromTo[i].AccountAlias {
				logger.Infof("Creating operation for account id: %s and account alias: %s", blc.ID, blc.Alias)

				preBalances = append(preBalances, blc)

				amt, bat, err := pkgTransaction.ValidateFromToOperation(fromTo[i], *validate, blc.ToTransactionBalance())
				if err != nil {
					libOpentelemetry.HandleSpanError(&span, "Failed to validate balances", err)

					logger.Errorf("Failed to validate balance: %v", err.Error())

					return nil, nil, err
				}

				if ops, handled := handler.tryBuildDoubleEntryOps(
					ctx, blc, fromTo[i], amt, bat, tran, transactionInput,
					transactionDate, isAnnotation, routeValidationEnabled, processedDoubleEntry,
				); handled {
					operations = append(operations, ops...)

					metricFactory.RecordTransactionProcessed(ctx, tran.OrganizationID, tran.LedgerID)

					continue
				}

				op := handler.buildStandardOp(
					blc, fromTo[i], amt, bat, tran, transactionInput, transactionDate, isAnnotation,
				)

				operations = append(operations, op)

				metricFactory.RecordTransactionProcessed(ctx, tran.OrganizationID, tran.LedgerID)
			}
		}
	}

	return operations, preBalances, nil
}

// propagateRouteValidation sets RouteValidationEnabled on Amount entries in the
// validate response maps when the transaction is pending or canceled. This flag controls how
// OperateBalances splits balance effects between Available and OnHold fields.
func propagateRouteValidation(ctx context.Context, validate *pkgTransaction.Responses, isPending bool, transactionStatus string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.propagate_route_validation")
	defer span.End()

	if validate == nil {
		return
	}

	isCanceled := transactionStatus == constant.CANCELED

	if !isPending && !isCanceled {
		return
	}

	count := 0

	for key, amt := range validate.From {
		amt.RouteValidationEnabled = true
		validate.From[key] = amt
		count++
	}

	logger.Infof("Propagated route validation to %d source entries (pending=%t, canceled=%t)", count, isPending, isCanceled)
}

// zeroAnnotationBalances zeroes out the Available, OnHold, and Version fields of the
// given balance and balanceAfter structs. It is used for annotation operations where
// we record the operation shape but do not reflect real balance values.
// Each call allocates fresh values so that callers never share pointers.
func zeroAnnotationBalances(balance, balanceAfter *operation.Balance) {
	aBefore := decimal.NewFromInt(0)
	balance.Available = &aBefore

	aAfter := decimal.NewFromInt(0)
	balanceAfter.Available = &aAfter

	oBefore := decimal.NewFromInt(0)
	balance.OnHold = &oBefore

	oAfter := decimal.NewFromInt(0)
	balanceAfter.OnHold = &oAfter

	vBefore := int64(0)
	balance.Version = &vBefore
	vAfter := int64(0)
	balanceAfter.Version = &vAfter
}

// buildDoubleEntryPendingOps generates two operations for a PENDING source entry
// when route validation is enabled:
// Op1: DEBIT (debit direction) - decreases Available only
// Op2: ONHOLD (credit direction) - increases OnHold only
// This ensures proper double-entry where each operation affects a single balance field.
func (handler *TransactionHandler) buildDoubleEntryPendingOps(
	ctx context.Context,
	blc *mmodel.Balance,
	ft pkgTransaction.FromTo,
	amt pkgTransaction.Amount,
	_ pkgTransaction.Balance,
	tran transaction.Transaction,
	transactionInput pkgTransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
) []*operation.Operation {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.build_double_entry_pending_ops")
	defer span.End()

	logger.Infof("Building double-entry ops for balance %s: DEBIT(debit) + ONHOLD(credit)", blc.ID)

	description := ft.Description
	if libCommons.IsNilOrEmpty(&ft.Description) {
		description = transactionInput.Description
	}

	// Op1: DEBIT (debit) - Available-- only
	// Balance before: original balance
	// Balance after: Available decreased, OnHold unchanged
	debitAvailable := blc.Available.Sub(amt.Value)
	debitVersion := blc.Version + 1

	debitBalance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &blc.OnHold,
		Version:   &blc.Version,
	}

	debitBalanceAfter := operation.Balance{
		Available: &debitAvailable,
		OnHold:    &blc.OnHold,
		Version:   &debitVersion,
	}

	if isAnnotation {
		zeroAnnotationBalances(&debitBalance, &debitBalanceAfter)
	}

	op1 := &operation.Operation{
		ID:              libCommons.GenerateUUIDv7().String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            constant.DEBIT,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          operation.Amount{Value: &amt.Value},
		Balance:         debitBalance,
		BalanceAfter:    debitBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
	}

	// Op2: ONHOLD (credit) - OnHold++ only
	// Balance before: op1's balance after (chaining)
	// Balance after: OnHold increased, Available unchanged from op1
	onholdOnHold := blc.OnHold.Add(amt.Value)
	onholdVersion := debitVersion + 1

	onholdBalance := operation.Balance{
		Available: &debitAvailable,
		OnHold:    &blc.OnHold,
		Version:   &debitVersion,
	}

	onholdBalanceAfter := operation.Balance{
		Available: &debitAvailable,
		OnHold:    &onholdOnHold,
		Version:   &onholdVersion,
	}

	if isAnnotation {
		zeroAnnotationBalances(&onholdBalance, &onholdBalanceAfter)
	}

	op2 := &operation.Operation{
		ID:              libCommons.GenerateUUIDv7().String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            libConstants.ONHOLD,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          operation.Amount{Value: &amt.Value},
		Balance:         onholdBalance,
		BalanceAfter:    onholdBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
	}

	return []*operation.Operation{op1, op2}
}

// buildDoubleEntryCanceledOps generates two operations for a CANCELED source entry
// when route validation is enabled:
// Op1: RELEASE (debit direction) - decreases OnHold only
// Op2: CREDIT (credit direction) - increases Available only
// This ensures proper double-entry where each operation affects a single balance field.
func (handler *TransactionHandler) buildDoubleEntryCanceledOps(
	ctx context.Context,
	blc *mmodel.Balance,
	ft pkgTransaction.FromTo,
	amt pkgTransaction.Amount,
	_ pkgTransaction.Balance,
	tran transaction.Transaction,
	transactionInput pkgTransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
) []*operation.Operation {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.build_double_entry_canceled_ops")
	defer span.End()

	logger.Infof("Building double-entry canceled ops for balance %s: RELEASE(debit) + CREDIT(credit)", blc.ID)

	description := ft.Description
	if libCommons.IsNilOrEmpty(&ft.Description) {
		description = transactionInput.Description
	}

	// Op1: RELEASE (debit) - OnHold-- only
	// Balance before: original balance
	// Balance after: OnHold decreased, Available unchanged
	releaseOnHold := blc.OnHold.Sub(amt.Value)
	releaseVersion := blc.Version + 1

	releaseBalance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &blc.OnHold,
		Version:   &blc.Version,
	}

	releaseBalanceAfter := operation.Balance{
		Available: &blc.Available,
		OnHold:    &releaseOnHold,
		Version:   &releaseVersion,
	}

	if isAnnotation {
		zeroAnnotationBalances(&releaseBalance, &releaseBalanceAfter)
	}

	op1 := &operation.Operation{
		ID:              libCommons.GenerateUUIDv7().String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            constant.RELEASE,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          operation.Amount{Value: &amt.Value},
		Balance:         releaseBalance,
		BalanceAfter:    releaseBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
	}

	// Op2: CREDIT (credit) - Available++ only
	// Balance before: op1's balance after (chaining)
	// Balance after: Available increased, OnHold unchanged from op1
	creditAvailable := blc.Available.Add(amt.Value)
	creditVersion := releaseVersion + 1

	creditBalance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &releaseOnHold,
		Version:   &releaseVersion,
	}

	creditBalanceAfter := operation.Balance{
		Available: &creditAvailable,
		OnHold:    &releaseOnHold,
		Version:   &creditVersion,
	}

	if isAnnotation {
		zeroAnnotationBalances(&creditBalance, &creditBalanceAfter)
	}

	op2 := &operation.Operation{
		ID:              libCommons.GenerateUUIDv7().String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            constant.CREDIT,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          operation.Amount{Value: &amt.Value},
		Balance:         creditBalance,
		BalanceAfter:    creditBalanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
	}

	return []*operation.Operation{op1, op2}
}

// tryBuildDoubleEntryOps checks whether the current balance entry qualifies for double-entry
// splitting (PENDING or CANCELED with route validation) and, if so, returns the pair of operations.
// Returns handled=true when operations were built, false when standard path should be used.
func (handler *TransactionHandler) tryBuildDoubleEntryOps(
	ctx context.Context,
	blc *mmodel.Balance,
	ft pkgTransaction.FromTo,
	amt pkgTransaction.Amount,
	bat pkgTransaction.Balance,
	tran transaction.Transaction,
	transactionInput pkgTransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
	routeValidationEnabled bool,
	processedDoubleEntry map[string]bool,
) ([]*operation.Operation, bool) {
	if !routeValidationEnabled || !ft.IsFrom {
		return nil, false
	}

	if !pkgTransaction.IsDoubleEntrySource(amt) {
		return nil, false
	}

	isPendingDoubleEntry := amt.TransactionType == constant.PENDING

	// Already processed this alias — skip without building duplicate ops
	if processedDoubleEntry[blc.Alias] {
		return nil, true
	}

	processedDoubleEntry[blc.Alias] = true

	if isPendingDoubleEntry {
		return handler.buildDoubleEntryPendingOps(
			ctx, blc, ft, amt, bat, tran, transactionInput, transactionDate, isAnnotation,
		), true
	}

	return handler.buildDoubleEntryCanceledOps(
		ctx, blc, ft, amt, bat, tran, transactionInput, transactionDate, isAnnotation,
	), true
}

// buildStandardOp creates a single operation for a standard (non-double-entry) balance entry.
func (handler *TransactionHandler) buildStandardOp(
	blc *mmodel.Balance,
	ft pkgTransaction.FromTo,
	amt pkgTransaction.Amount,
	bat pkgTransaction.Balance,
	tran transaction.Transaction,
	transactionInput pkgTransaction.Transaction,
	transactionDate time.Time,
	isAnnotation bool,
) *operation.Operation {
	amount := operation.Amount{
		Value: &amt.Value,
	}

	balance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &blc.OnHold,
		Version:   &blc.Version,
	}

	balanceAfter := operation.Balance{
		Available: &bat.Available,
		OnHold:    &bat.OnHold,
		Version:   &bat.Version,
	}

	if isAnnotation {
		zeroAnnotationBalances(&balance, &balanceAfter)
	}

	description := ft.Description
	if libCommons.IsNilOrEmpty(&ft.Description) {
		description = transactionInput.Description
	}

	return &operation.Operation{
		ID:              libCommons.GenerateUUIDv7().String(),
		TransactionID:   tran.ID,
		Description:     description,
		Type:            amt.Operation,
		AssetCode:       transactionInput.Send.Asset,
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          amount,
		Balance:         balance,
		BalanceAfter:    balanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
		BalanceKey:      blc.Key,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		CreatedAt:       transactionDate,
		UpdatedAt:       time.Now(),
		Route:           ft.Route,
		RouteID:         ft.RouteID,
		Metadata:        ft.Metadata,
		BalanceAffected: !isAnnotation,
	}
}

// createTransaction func that received struct from DSL parsed and create Transaction
func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, transactionInput pkgTransaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	parentID, _ := c.Locals("transaction_id").(uuid.UUID)
	transactionID := libCommons.GenerateUUIDv7()

	c.Set(libConstants.IdempotencyReplayed, "false")

	transactionDate, err := handler.checkTransactionDate(logger, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to check transaction date", err)

		logger.Errorf("Failed to check transaction date: %v", err)

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", transactionInput)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction to JSON string", err)

		logger.Errorf("Failed to convert transaction to JSON string, Error: %s", err.Error())
	}

	if transactionInput.Send.Value.LessThanOrEqual(decimal.Zero) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid transaction with non-positive value", err)

		logger.Warnf("Transaction value must be greater than zero")

		return http.WithError(c, err)
	}

	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, true)...)
	to := handler.HandleAccountFields(transactionInput.Send.Distribute.To, true)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	ctxIdempotency, spanIdempotency := tracer.Start(ctx, "handler.create_transaction_idempotency")

	ts, _ := libCommons.StructToJSONString(transactionInput)
	hash := libCommons.HashSHA256(ts)
	key, ttl := http.GetIdempotencyKeyAndTTL(c)

	value, internalKey, err := handler.Command.CreateOrCheckIdempotencyKey(ctxIdempotency, organizationID, ledgerID, key, hash, ttl)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanIdempotency, "Error on create or check redis idempotency key", err)
		spanIdempotency.End()

		logger.Infof("Error on create or check redis idempotency key: %v", err.Error())

		return http.WithError(c, err)
	} else if !libCommons.IsNilOrEmpty(value) {
		t := transaction.Transaction{}
		if err = json.Unmarshal([]byte(*value), &t); err != nil {
			libOpentelemetry.HandleSpanError(&spanIdempotency, "Error to deserialization idempotency transaction json on redis", err)

			logger.Errorf("Error to deserialization idempotency transaction json on redis: %v", err)
			spanIdempotency.End()

			return http.WithError(c, err)
		}

		spanIdempotency.End()
		c.Set(libConstants.IdempotencyReplayed, "true")

		return http.Created(c, t)
	}

	spanIdempotency.End()

	validate, err := pkgTransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate send source and distribute", err)

		logger.Warnf("Failed to validate send source and distribute: %v", err.Error())

		err = pkg.HandleKnownBusinessValidationErrors(err)

		_ = handler.Command.RedisRepo.Del(ctx, *internalKey)

		return http.WithError(c, err)
	}

	ledgerSettings := handler.Query.GetLedgerSettings(ctx, organizationID, ledgerID)
	if ledgerSettings.Accounting.ValidateRoutes {
		propagateRouteValidation(ctx, validate, transactionInput.Pending, transactionStatus)
	}

	ctxSendTransactionToRedisQueue, spanSendTransactionToRedisQueue := tracer.Start(ctx, "handler.create_transaction.send_transaction_to_redis_queue")

	err = handler.Command.SendTransactionToRedisQueue(ctxSendTransactionToRedisQueue, organizationID, ledgerID, transactionID, transactionInput, validate, transactionStatus, transactionDate, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendTransactionToRedisQueue, "Failed to send transaction to backup cache", err)

		logger.Errorf("Failed to send transaction to backup cache: %v", err.Error())

		// Only delete idempotency key if marshal failed
		// If Redis is down, Del would also fail
		if errors.Is(err, constant.ErrTransactionBackupCacheMarshalFailed) {
			_ = handler.Command.RedisRepo.Del(ctxSendTransactionToRedisQueue, *internalKey)
		}

		spanSendTransactionToRedisQueue.End()

		return http.WithError(c, pkg.ValidateBusinessError(err, reflect.TypeOf(transaction.Transaction{}).Name()))
	}

	spanSendTransactionToRedisQueue.End()

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")

	balancesBefore, balancesAfter, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, transactionID, &transactionInput, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanGetBalances, "Failed to get balances", err)

		logger.Errorf("Failed to get balances: %v", err.Error())

		_ = handler.Command.RedisRepo.Del(ctx, *internalKey)

		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, organizationID, ledgerID, transactionID.String())
		spanGetBalances.End()

		return http.WithError(c, err)
	}

	spanGetBalances.End()

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, false)...)
	to = handler.HandleAccountFields(transactionInput.Send.Distribute.To, false)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	var parentTransactionID *string

	if parentID != uuid.Nil {
		str := parentID.String()
		parentTransactionID = &str
	}

	tran := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              transactionInput.Description,
		Amount:                   &transactionInput.Send.Value,
		AssetCode:                transactionInput.Send.Asset,
		ChartOfAccountsGroupName: transactionInput.ChartOfAccountsGroupName,
		CreatedAt:                transactionDate,
		UpdatedAt:                time.Now(),
		Route:                    transactionInput.Route,
		Metadata:                 transactionInput.Metadata,
		Status: transaction.Status{
			Code:        transactionStatus,
			Description: &transactionStatus,
		},
	}

	operations, _, err := handler.BuildOperations(ctx, balancesBefore, fromTo, transactionInput, *tran, validate, transactionDate, transactionStatus == constant.NOTED)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate balances", err)

		logger.Errorf("Failed to validate balance: %v", err.Error())

		return http.WithError(c, err)
	}

	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
	tran.Operations = operations

	handler.Command.UpdateTransactionBackupOperations(ctx, organizationID, ledgerID, transactionID.String(), operations)

	// Promote status to APPROVED for write-behind and RabbitMQ so the cache and consumer
	// already have the final status. The original status is restored before the HTTP response
	// to preserve the current API contract (clients see CREATED).
	originalStatus := tran.Status

	if transactionStatus == constant.CREATED {
		approved := constant.APPROVED
		tran.Status = transaction.Status{Code: approved, Description: &approved}
	}

	handler.Command.CreateWriteBehindTransaction(ctx, organizationID, ledgerID, tran, transactionInput)

	err = handler.Command.WriteTransaction(ctx, organizationID, ledgerID, &transactionInput, validate, balancesBefore, balancesAfter, tran)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "failed to update BTO", err)

		logger.Errorf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err)

		return http.WithError(c, err)
	}

	tran.Status = originalStatus

	go handler.Command.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, key, hash, *tran, ttl)

	go handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())

	return http.Created(c, tran)
}

// commitOrCancelTransaction func that is responsible to commit or cancel transaction.
func (handler *TransactionHandler) commitOrCancelTransaction(c *fiber.Ctx, tran *transaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.commit_or_cancel_transaction")
	defer span.End()

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)

	lockPendingTransactionKey := utils.PendingTransactionLockKey(organizationID, ledgerID, tran.ID)

	// TTL is of 300 seconds (time.Seconds is set inside the SetNX method)
	ttl := time.Duration(300)

	success, err := handler.Command.RedisRepo.SetNX(ctx, lockPendingTransactionKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set on redis", err)

		logger.Errorf("Failed to set on redis: %v", err)

		return http.WithError(c, err)
	}

	// if it could not acquire the lock, the pending transaction is already being processed
	if !success {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction is locked", err)

		logger.Errorf("Failed, Transaction: %s is locked, Error: %s", tran.ID, err.Error())

		return http.WithError(c, err)
	}

	// deleteLockOnError removes the lock only when an error occurs, allowing immediate retry.
	// On success, the lock remains until TTL expires to prevent duplicate processing.
	deleteLockOnError := func() {
		if delErr := handler.Command.RedisRepo.Del(ctx, lockPendingTransactionKey); delErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete pending transaction lock", delErr)

			logger.Errorf("Failed to delete pending transaction lock key: %v", delErr)
		}
	}

	transactionInput := tran.Body

	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Source.From)
	handler.ApplyDefaultBalanceKeys(transactionInput.Send.Distribute.To)

	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, true)...)
	to := handler.HandleAccountFields(transactionInput.Send.Distribute.To, true)

	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	if tran.Status.Code != constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction is not pending", err)

		logger.Errorf("Failed, Transaction: %s is not pending, Error: %s", tran.ID, err.Error())

		deleteLockOnError()

		return http.WithError(c, err)
	}

	validate, err := pkgTransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate send source and distribute", err)

		logger.Error("Failed to validate send source and distribute: %v", err.Error())

		err = pkg.HandleKnownBusinessValidationErrors(err)

		deleteLockOnError()

		return http.WithError(c, err)
	}

	ledgerSettings := handler.Query.GetLedgerSettings(ctx, organizationID, ledgerID)
	if ledgerSettings.Accounting.ValidateRoutes {
		propagateRouteValidation(ctx, validate, transactionInput.Pending, transactionStatus)
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")

	balancesBefore, balancesAfter, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, tran.IDtoUUID(), nil, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanGetBalances, "Failed to get balances", err)

		logger.Errorf("Failed to get balances: %v", err.Error())
		spanGetBalances.End()

		deleteLockOnError()

		return http.WithError(c, err)
	}

	fromTo = append(fromTo, handler.HandleAccountFields(transactionInput.Send.Source.From, false)...)
	to = handler.HandleAccountFields(transactionInput.Send.Distribute.To, false)

	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	tran.UpdatedAt = time.Now()
	tran.Status = transaction.Status{
		Code:        transactionStatus,
		Description: &transactionStatus,
	}

	operations, preBalances, err := handler.BuildOperations(ctx, balancesBefore, fromTo, transactionInput, *tran, validate, time.Now(), false)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate balances", err)

		logger.Errorf("Failed to validate balance: %v", err.Error())

		return http.WithError(c, err)
	}

	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
	tran.Operations = operations

	// Send commit/cancel transaction to Redis backup queue for recovery
	ctxBackup, spanBackup := tracer.Start(ctx, "handler.commit_or_cancel_transaction.send_to_redis_queue")

	if backupErr := handler.Command.SendTransactionToRedisQueue(ctxBackup, organizationID, ledgerID, tran.IDtoUUID(), transactionInput, validate, transactionStatus, time.Now(), preBalances); backupErr != nil {
		libOpentelemetry.HandleSpanError(&spanBackup, "Failed to send transaction to backup cache", backupErr)

		logger.Warnf("Failed to send commit/cancel transaction to backup cache: %v", backupErr)
	}

	spanBackup.End()

	// immediately update transaction status if application is configured to persist transaction asynchronously
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		_, err = handler.Command.UpdateTransactionStatus(ctx, tran)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction status synchronously", err)

			logger.Errorf("Failed to update transaction status synchronously for Transaction: %s, Error: %s", tran.ID, err.Error())
		}
	}

	err = handler.Command.WriteTransaction(ctx, organizationID, ledgerID, &transactionInput, validate, preBalances, balancesAfter, tran)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "failed to update BTO", err)

		logger.Errorf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err)

		return http.WithError(c, err)
	}

	go handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		go handler.Command.UpdateWriteBehindTransaction(context.Background(), organizationID, ledgerID, tran)
	}

	return http.Created(c, tran)
}

// ApplyDefaultBalanceKeys sets the BalanceKey to "default" for any entries where the BalanceKey is empty.
func (handler *TransactionHandler) ApplyDefaultBalanceKeys(entries []pkgTransaction.FromTo) {
	for i := range entries {
		if entries[i].BalanceKey == "" {
			entries[i].BalanceKey = constant.DefaultBalanceKey
		}
	}
}

// getAliasWithoutKey takes a slice of strings and returns a new slice containing the first part of each string split by '#'.
func getAliasWithoutKey(array []string) []string {
	result := make([]string, len(array))

	for i, str := range array {
		parts := strings.Split(str, "#")
		result[i] = parts[0]
	}

	return result
}
>>>>>>> 5745e5329 (feat(transaction): add direction preservation and bidirectional route validation for reverts :sparkles:)
