// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	goldTransaction "github.com/LerianStudio/midaz/v4/pkg/gold/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
	// FeeApplier drives the in-process fee engine inside the create seam. It is
	// injected at bootstrap from the fee use case; a nil applier disables fee
	// application (the create path stays unchanged).
	FeeApplier FeeApplier
	// TracerReserver drives the tracer two-phase reservation lifecycle from the
	// create seam. It is injected at bootstrap from the tracer HTTP client; a
	// nil reserver means the tracer integration is disabled (the create path
	// stays unchanged). The per-ledger tracer.mode gate lives at the call site.
	TracerReserver TracerReserver
	// FeesMongoManager resolves the CURRENT tenant's fee Mongo database at the
	// fee seam when MultiTenantEnabled is true. The fee pack/billing repos read
	// the GENERIC tmcore MB key, which the route-scoped feesTenantMiddleware
	// only sets on FEE routes — never on the transaction route — so the seam
	// must resolve and inject it onto a derived ctx itself. Nil in single-tenant
	// mode (and in tests that do not exercise the seam).
	FeesMongoManager feesDBResolver
	// MultiTenantEnabled gates the fee-seam tenant resolution. When false the
	// static fee connection is correct and resolveFeesTenantContext is a no-op.
	MultiTenantEnabled bool
}

// CreateTransactionJSON method that create transaction using JSON
//
//	@Summary		Create a Transaction using JSON
//	@Description	Creates a full double-entry transaction by specifying explicit source accounts (send.source.from) and destination accounts (send.distribute.to). Both sides of the ledger entry must be provided. Supports pending-hold semantics via the 'pending' flag, idempotency via the Idempotency-Key header, and optional fee application. An optional 'skip' object (see TransactionSkip) carries per-call control opt-outs: skip.fees bypasses fee computation and skip.tracer bypasses the tracer reserve. A skip is honored only when the ledger opts into it via the matching override (overrides.allowFeeSkip / overrides.allowTracerSkip); a skip requested without the override is rejected with HTTP 422 (error 0490, ErrSkipNotPermitted). On an idempotency replay the first transaction's outcome is returned and a replayer's differing skip is ignored.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction		body		mtransaction.CreateTransactionInput	true	"Full transaction input with explicit source and destination accounts"
//	@Success		201				{object}	Transaction							"Successfully created transaction"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Organization, ledger, or account not found"
//	@Failure		409				{object}	mmodel.Error						"Duplicate idempotency key"
//	@Failure		422				{object}	mmodel.Error						"Unprocessable entity: insufficient funds, account ineligible, or transaction value mismatch"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Failure		503				{object}	mmodel.Error						"Usage-limit service temporarily unavailable"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json [post]
func (handler *TransactionHandler) CreateTransactionJSON(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*mtransaction.CreateTransactionInput)
	transactionInput := input.BuildTransaction()

	logSafePayload(ctx, logger, "Request to create a transaction", transactionInput)
	recordSafePayloadAttributes(span, transactionInput)

	return handler.createTransaction(c, *transactionInput, transactionInput.InitialStatus())
}

// CreateTransactionAnnotation method that create transaction using JSON
//
//	@Summary		Create a Transaction Annotation using JSON
//	@Description	Creates an annotation-only transaction that records a memo/audit entry. The transaction is persisted with status NOTED and applies no balance changes; source and destination accounts are recorded for reference only.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			transaction		body		mtransaction.CreateTransactionInput	true	"Transaction input; source and destination accounts are recorded but no balance changes are applied"
//	@Success		201				{object}	Transaction							"Successfully created annotation transaction"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Organization, ledger, or account not found"
//	@Failure		409				{object}	mmodel.Error						"Duplicate idempotency key"
//	@Failure		422				{object}	mmodel.Error						"Unprocessable entity: insufficient funds, account ineligible, or transaction value mismatch"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Failure		503				{object}	mmodel.Error						"Usage-limit service temporarily unavailable"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/annotation [post]
func (handler *TransactionHandler) CreateTransactionAnnotation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_annotation")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*mtransaction.CreateTransactionInput)
	transactionInput := input.BuildTransaction()

	logSafePayload(ctx, logger, "Create a transaction annotation without an affected balance", transactionInput)
	recordSafePayloadAttributes(span, transactionInput)

	return handler.createTransaction(c, *transactionInput, constant.NOTED)
}

// CreateTransactionInflow method that creates a transaction without specifying a source
//
//	@Summary		Create a Transaction without passing from source
//	@Description	Creates a transaction where funds flow INTO destination accounts without an explicit source; the source is auto-resolved to the external/system account. Use for external receipts, deposits, and credits. An optional 'skip' object (see TransactionSkip) carries per-call control opt-outs: skip.fees bypasses fee computation and skip.tracer bypasses the tracer reserve. A skip is honored only when the ledger opts into it via the matching override (overrides.allowFeeSkip / overrides.allowTracerSkip); a skip requested without the override is rejected with HTTP 422 (error 0490, ErrSkipNotPermitted).
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string										false	"Request ID for tracing"
//	@Param			organization_id	path		string										true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string										true	"Ledger ID in UUID format"
//	@Param			transaction		body		mtransaction.CreateTransactionInflowInput	true	"Inflow transaction input specifying only destination accounts; source is resolved automatically"
//	@Success		201				{object}	Transaction									"Successfully created inflow transaction"
//	@Failure		400				{object}	mmodel.Error								"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error								"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error								"Forbidden access"
//	@Failure		404				{object}	mmodel.Error								"Organization, ledger, or destination account not found"
//	@Failure		409				{object}	mmodel.Error								"Duplicate idempotency key"
//	@Failure		422				{object}	mmodel.Error								"Unprocessable entity: insufficient funds, account ineligible, or transaction value mismatch"
//	@Failure		500				{object}	mmodel.Error								"Internal server error"
//	@Failure		503				{object}	mmodel.Error								"Usage-limit service temporarily unavailable"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/inflow [post]
func (handler *TransactionHandler) CreateTransactionInflow(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_inflow")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*mtransaction.CreateTransactionInflowInput)
	transactionInput := input.BuildInflowEntry()

	logSafePayload(ctx, logger, "Request to create a transaction inflow", transactionInput)
	recordSafePayloadAttributes(span, transactionInput)

	return handler.createTransaction(c, *transactionInput, transactionInput.InitialStatus())
}

// CreateTransactionOutflow method that creates a transaction without specifying a distribution
//
//	@Summary		Create a Transaction without passing to distribution
//	@Description	Creates a transaction where funds flow OUT of source accounts without an explicit destination; the destination is auto-resolved. Use for withdrawals, payments, and debits. An optional 'skip' object (see TransactionSkip) carries per-call control opt-outs: skip.fees bypasses fee computation and skip.tracer bypasses the tracer reserve. A skip is honored only when the ledger opts into it via the matching override (overrides.allowFeeSkip / overrides.allowTracerSkip); a skip requested without the override is rejected with HTTP 422 (error 0490, ErrSkipNotPermitted).
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string										false	"Request ID for tracing"
//	@Param			organization_id	path		string										true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string										true	"Ledger ID in UUID format"
//	@Param			transaction		body		mtransaction.CreateTransactionOutflowInput	true	"Outflow transaction input specifying only source accounts; destination is resolved automatically"
//	@Success		201				{object}	Transaction									"Successfully created outflow transaction"
//	@Failure		400				{object}	mmodel.Error								"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error								"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error								"Forbidden access"
//	@Failure		404				{object}	mmodel.Error								"Organization, ledger, or source account not found"
//	@Failure		409				{object}	mmodel.Error								"Duplicate idempotency key"
//	@Failure		422				{object}	mmodel.Error								"Unprocessable entity: insufficient funds or account ineligible"
//	@Failure		500				{object}	mmodel.Error								"Internal server error"
//	@Failure		503				{object}	mmodel.Error								"Usage-limit service temporarily unavailable"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/outflow [post]
func (handler *TransactionHandler) CreateTransactionOutflow(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_outflow")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*mtransaction.CreateTransactionOutflowInput)
	transactionInput := input.BuildOutflowEntry()

	logSafePayload(ctx, logger, "Request to create a transaction outflow", transactionInput)
	recordSafePayloadAttributes(span, transactionInput)

	return handler.createTransaction(c, *transactionInput, transactionInput.InitialStatus())
}

// CreateTransactionDSL method that create transaction using DSL
//
//	@Summary		Create a Transaction using DSL
//	@Description	Uploads a Gold DSL (.casl) multipart file that is parsed, validated, then executed as a transaction. The DSL grammar carries no per-call skip, so fee and tracer controls always run on this path. DEPRECATED: use POST /transactions/json instead. Sunset 2026-08-01.
//	@Deprecated		true
//	@Tags			Transactions
//	@Accept			mpfd
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			transaction		formData	file						true	"Transaction DSL file (Gold .casl format)"
//	@Success		201				{object}	Transaction					"Successfully created transaction from DSL"
//	@Failure		400				{object}	mmodel.Error				"Invalid DSL file format or validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Organization, ledger, or account not found"
//	@Failure		409				{object}	mmodel.Error				"Duplicate idempotency key"
//	@Failure		422				{object}	mmodel.Error				"Unprocessable entity: insufficient funds, account ineligible, or transaction value mismatch"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Failure		503				{object}	mmodel.Error				"Usage-limit service temporarily unavailable"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl [post]
func (handler *TransactionHandler) CreateTransactionDSL(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_dsl")
	defer span.End()

	c.SetUserContext(ctx)

	c.Set("Deprecation", "true")
	c.Set("Sunset", "Sat, 01 Aug 2026 00:00:00 GMT")
	c.Set("Link", "</v1/organizations/"+c.Params("organization_id")+
		"/ledgers/"+c.Params("ledger_id")+
		"/transactions/json>; rel=\"successor-version\"")

	logger.Log(ctx, libLog.LevelWarn, "DEPRECATED ENDPOINT: POST /transactions/dsl called, use POST /transactions/json instead",
		libLog.String("request_id", c.Get("X-Request-Id")),
		libLog.String("sunset_date", "2026-08-01"),
	)

	_, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate query parameters", libLog.Err(err))

		return http.WithError(c, err)
	}

	dsl, err := http.GetFileFromHeader(c)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get file from Header", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to get file from header", libLog.Err(err))

		return http.WithError(c, err)
	}

	errListener := goldTransaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, constant.EntityTransaction, errListener.Errors)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate script in DSL", err)

		return http.WithError(c, err)
	}

	parsed := goldTransaction.Parse(dsl)

	transactionInput, ok := parsed.(mtransaction.Transaction)
	if !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, constant.EntityTransaction)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse script in DSL", err)

		return http.WithError(c, err)
	}

	recordSafePayloadAttributes(span, transactionInput.Send)

	return handler.createTransaction(c, transactionInput, transactionInput.InitialStatus())
}

// GetTransaction method that get transaction created before
//
//	@Summary		Get a Transaction by ID
//	@Description	Retrieves a transaction by UUID, including its operations. Reads cache-first and falls back to the database.
//	@Tags			Transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			transaction_id	path		string						true	"Transaction ID in UUID format"
//	@Success		200				{object}	Transaction					"Successfully retrieved transaction with operations"
//	@Failure		400				{object}	mmodel.Error				"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Transaction not found"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [get]
func (handler *TransactionHandler) GetTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction")
	defer span.End()

	params, err := readPathParams(c)
	if err != nil {
		return http.WithError(c, err)
	}

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to validate query parameters", libLog.Err(err))

		return http.WithError(c, err)
	}

	if wbTran, wbErr := handler.Query.GetWriteBehindTransaction(ctx, params.OrganizationID, params.LedgerID, params.TransactionID); wbErr == nil {
		c.Set("X-Cache-Hit", "true")

		return http.OK(c, wbTran)
	} else {
		logger.Log(ctx, libLog.LevelDebug, "Write-behind cache miss, falling back to database",
			libLog.String("transaction_id", params.TransactionID.String()), libLog.Err(wbErr))
	}

	c.Set("X-Cache-Hit", "false")

	tran, err := handler.Query.GetTransactionByID(ctx, params.OrganizationID, params.LedgerID, params.TransactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to retrieve transaction",
			libLog.String("transaction_id", params.TransactionID.String()), libLog.Err(err))

		return http.WithError(c, err)
	}

	ctxGetTransaction, spanGetTransaction := tracer.Start(ctx, "handler.get_transaction.get_operations")
	defer spanGetTransaction.End()

	headerParams.Metadata = &bson.M{}

	tran, err = handler.Query.GetOperationsByTransaction(ctxGetTransaction, params.OrganizationID, params.LedgerID, tran, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanGetTransaction, "Failed to retrieve Operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to retrieve operations",
			libLog.String("transaction_id", params.TransactionID.String()), libLog.Err(err))

		return http.WithError(c, err)
	}

	return http.OK(c, tran)
}
