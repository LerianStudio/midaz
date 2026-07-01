// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
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

	return handler.createTransactionFiber(c, *transactionInput, transactionInput.InitialStatus(), false)
}

// CreateTransactionAnnotation method that create transaction using JSON
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

	return handler.createTransactionFiber(c, *transactionInput, constant.NOTED, false)
}

// CreateTransactionInflow method that creates a transaction without specifying a source
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

	return handler.createTransactionFiber(c, *transactionInput, transactionInput.InitialStatus(), false)
}

// CreateTransactionOutflow method that creates a transaction without specifying a distribution
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

	return handler.createTransactionFiber(c, *transactionInput, transactionInput.InitialStatus(), false)
}

// CreateTransactionDSL method that create transaction using DSL
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

	return handler.createTransactionFiber(c, transactionInput, transactionInput.InitialStatus(), false)
}

// GetTransaction method that get transaction created before
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

	headerParams.Metadata = &bson.M{}

	tran, cacheHit, err := handler.getTransaction(ctx, params.OrganizationID, params.LedgerID, params.TransactionID, headerParams)
	if err != nil {
		return http.WithError(c, err)
	}

	if cacheHit {
		c.Set("X-Cache-Hit", "true")
	} else {
		c.Set("X-Cache-Hit", "false")
	}

	return http.OK(c, tran)
}

// getTransaction is the transport-neutral read core. It reads write-behind cache first
// (returning cacheHit=true, operations already materialized in the cached shape), and on
// a miss falls back to the DB then materializes operations via GetOperationsByTransaction.
// The caller sets the X-Cache-Hit response header off the returned flag. headerParams is
// expected to already carry the Metadata reset the Fiber path applied.
func (handler *TransactionHandler) getTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, headerParams *http.QueryHeader) (*transaction.Transaction, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction.core")
	defer span.End()

	if wbTran, wbErr := handler.Query.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID); wbErr == nil {
		return wbTran, true, nil
	} else {
		logger.Log(ctx, libLog.LevelDebug, "Write-behind cache miss, falling back to database",
			libLog.String("transaction_id", transactionID.String()), libLog.Err(wbErr))
	}

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to retrieve transaction",
			libLog.String("transaction_id", transactionID.String()), libLog.Err(err))

		return nil, false, err
	}

	ctxGetTransaction, spanGetTransaction := tracer.Start(ctx, "handler.get_transaction.get_operations")
	defer spanGetTransaction.End()

	tran, err = handler.Query.GetOperationsByTransaction(ctxGetTransaction, organizationID, ledgerID, tran, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanGetTransaction, "Failed to retrieve Operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to retrieve operations",
			libLog.String("transaction_id", transactionID.String()), libLog.Err(err))

		return nil, false, err
	}

	return tran, false, nil
}
