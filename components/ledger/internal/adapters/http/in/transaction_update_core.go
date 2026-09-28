// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// updateTransaction is the transport-neutral update core: it records the safe payload
// shape on the span, runs command.UpdateTransaction, then re-reads the transaction via query.GetTransactionByID
// (mutable fields only — amounts/accounts/status are immutable).
func (handler *TransactionHandler) updateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, payload *transaction.UpdateTransactionInput) (*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	_, err := handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update transaction on command", err)

		return nil, err
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve transaction on query", err)

		return nil, err
	}

	return trans, nil
}
