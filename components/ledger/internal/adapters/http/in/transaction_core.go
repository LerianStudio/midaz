// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
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

// buildOverriddenTransaction builds the transaction from the input, forces
// Pending=false (so InitialStatus resolves to non-pending), and stamps the
// given OperationTypeOverride.
func (handler *TransactionHandler) buildOverriddenTransaction(input *mtransaction.CreateTransactionInput, operationType string) mtransaction.Transaction {
	transactionInput := input.BuildTransaction()
	transactionInput.Pending = false
	transactionInput.OperationTypeOverride = operationType

	return *transactionInput
}

// getTransaction is the transport-neutral read core. It reads write-behind cache first
// (returning cacheHit=true, operations already materialized in the cached shape), and on
// a miss falls back to the DB then materializes operations via GetOperationsByTransaction.
// The caller sets the X-Cache-Hit response header off the returned flag and is expected
// to have already applied the Metadata reset to headerParams.
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

	_, spanGetTransaction := tracer.Start(ctx, "handler.get_transaction.get_operations")
	defer spanGetTransaction.End()

	tran, err = handler.Query.GetOperationsByTransaction(ctx, organizationID, ledgerID, tran, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanGetTransaction, "Failed to retrieve Operations", err)
		logger.Log(ctx, libLog.LevelError, "Failed to retrieve operations",
			libLog.String("transaction_id", transactionID.String()), libLog.Err(err))

		return nil, false, err
	}

	return tran, false, nil
}
