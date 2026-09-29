// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// collectFeeDebt runs one standalone collection of a debtor balance's open fee debts.
// No span attribute carries the alias or an amount.
func (handler *TransactionHandler) collectFeeDebt(ctx context.Context, organizationID, ledgerID uuid.UUID, accountAlias, balanceKey string, maxAmount *decimal.Decimal) (*FeeDebtCollectOutput, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.collect_fee_debt")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	result, err := handler.Command.CollectFeeDebt(ctx, command.CollectFeeDebtInput{
		OrganizationID: organizationID, LedgerID: ledgerID,
		BalanceRef: mtransaction.AliasKey(accountAlias, balanceKey), MaxAmount: maxAmount,
	})
	if err != nil {
		handleSpanByErrorClass(span, "Failed to collect fee debt", err)

		return nil, err
	}

	out := &FeeDebtCollectOutput{Collected: result.Collected}
	if result.Transaction != nil {
		out.TransactionID = &result.Transaction.ID
	}

	return out, nil
}
