// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	goldTransaction "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/gofiber/fiber/v2"
)

// CreateTransactionAnnotation method that create transaction using JSON
//
//	@Summary		Create a Transaction Annotation using JSON
//	@Description	Create a Transaction Annotation with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionInput	true	"Transaction Input"
//	@Success		201				{object}	Transaction
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

	input := p.(*pkgTransaction.CreateTransactionInput)
	transactionInput := input.BuildTransaction()
	logSafePayload(ctx, logger, "Create a transaction annotation without an affected balance", transactionInput)
	recordSafePayloadAttributes(span, transactionInput)

	return handler.createTransaction(c, *transactionInput, constant.NOTED)
}

// CreateTransactionInflow method that creates a transaction without specifying a source
//
//	@Summary		Create a Transaction without passing from source
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string							false	"Request ID"
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionInflowInput	true	"Transaction Input"
//	@Success		201				{object}	Transaction
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

	input := p.(*pkgTransaction.CreateTransactionInflowInput)
	transactionInput := input.BuildInflowEntry()
	logSafePayload(ctx, logger, "Request to create a transaction inflow", transactionInput)

	recordSafePayloadAttributes(span, transactionInput)

	return handler.createTransaction(c, *transactionInput, transactionInput.InitialStatus())
}

// CreateTransactionOutflow method that creates a transaction without specifying a distribution
//
//	@Summary		Create a Transaction without passing to distribution
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionOutflowInput	true	"Transaction Input"
//	@Success		201				{object}	Transaction
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

	input := p.(*pkgTransaction.CreateTransactionOutflowInput)
	transactionInput := input.BuildOutflowEntry()
	logSafePayload(ctx, logger, "Request to create a transaction outflow", transactionInput)

	recordSafePayloadAttributes(span, transactionInput)

	return handler.createTransaction(c, *transactionInput, transactionInput.InitialStatus())
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
//	@Success		200				{object}	Transaction
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

	c.Set("Deprecation", "true")
	c.Set("Sunset", "Sat, 01 Aug 2026 00:00:00 GMT")
	c.Set("Link", "</v1/organizations/"+c.Params("organization_id")+
		"/ledgers/"+c.Params("ledger_id")+
		"/transactions/json>; rel=\"successor-version\"")

	logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("DEPRECATED ENDPOINT: POST /transactions/dsl called. "+
		"request_id=%s. Use POST /transactions/json instead. "+
		"Sunset date: 2026-08-01", c.Get("X-Request-Id")))

	_, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate query parameters: %s", err.Error()))

		return http.WithError(c, err)
	}

	dsl, err := http.GetFileFromHeader(c)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get file from Header", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get file from Header: %s", err.Error()))

		return http.WithError(c, err)
	}

	errListener := goldTransaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name(), errListener.Errors)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate script in DSL", err)

		return http.WithError(c, err)
	}

	parsed := goldTransaction.Parse(dsl)

	transactionInput, ok := parsed.(pkgTransaction.Transaction)
	if !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse script in DSL", err)

		return http.WithError(c, err)
	}

	recordSafePayloadAttributes(span, transactionInput.Send)

	return handler.createTransaction(c, transactionInput, transactionInput.InitialStatus())
}
