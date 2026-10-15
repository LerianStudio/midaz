// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// buildOperationsCallerSpan stands in for the command span that calls BuildOperations.
const buildOperationsCallerSpan = "command.create_transaction_v1"

// removedBuildOperationsSpans are the operation-building steps that no longer open a span.
var removedBuildOperationsSpans = []string{
	"command.build_transaction_operations",
	"command.build_double_entry_pending_ops",
	"command.build_double_entry_canceled_ops",
}

// A pending double-entry build runs BuildOperations and buildDoubleEntryPendingOps under
// the caller's span without opening a span of either, and the route-validation flag lands
// on the caller's span.
func TestBuildTransactionOperationsSpans_PendingDoubleEntryOpensNoSpan(t *testing.T) {
	transactionDate := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	balance := &mmodel.Balance{
		ID:             "balance-default",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		AccountID:      "account-1",
		Alias:          "0#@sender#default",
		Key:            constant.DefaultBalanceKey,
		Available:      decimal.NewFromInt(500),
		OnHold:         decimal.Zero,
		Version:        1,
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.Zero,
	}
	amount := mtransaction.Amount{
		Asset:                  "BRL",
		Value:                  decimal.NewFromInt(100),
		Operation:              constant.ONHOLD,
		TransactionType:        constant.PENDING,
		RouteValidationEnabled: true,
	}
	fromTo := []mtransaction.FromTo{{
		AccountAlias: balance.Alias,
		BalanceKey:   constant.DefaultBalanceKey,
		Amount:       &amount,
		IsFrom:       true,
	}}
	validate := &mtransaction.Responses{
		From:    map[string]mtransaction.Amount{balance.Alias: amount},
		Sources: []string{"@sender#default"},
		Aliases: []string{"@sender#default"},
	}
	tran := transaction.Transaction{ID: "txn-1", OrganizationID: "org-1", LedgerID: "ledger-1"}
	input := mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: "BRL"}}

	ctx, recorder := spantest.RecordingContext()
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, parent := tracer.Start(ctx, buildOperationsCallerSpan)

	ops, _, err := (&UseCase{}).BuildOperations(ctx, []*mmodel.Balance{balance}, nil, fromTo,
		input, tran, validate, transactionDate, false, true, nil, constant.ActionHold)

	parent.End()

	require.NoError(t, err)
	require.Len(t, ops, 2, "the pending double-entry path must have run")

	spantest.AssertNoSpan(t, recorder, removedBuildOperationsSpans...)
	assert.Equal(t, []string{buildOperationsCallerSpan}, spantest.SpanNames(recorder))

	span := spantest.FindSpan(t, recorder, buildOperationsCallerSpan)
	assert.True(t, spantest.BoolAttrs(span)["app.route_validation_enabled"],
		"the route-validation flag moves to the caller's span")
	assert.Equal(t, codes.Unset, span.Status().Code)
}

// The canceled double-entry builder opens no span either.
func TestBuildTransactionOperationsSpans_CanceledDoubleEntryOpensNoSpan(t *testing.T) {
	blc := &mmodel.Balance{
		ID:        "balance-1",
		Alias:     "@sender",
		Key:       constant.DefaultBalanceKey,
		Available: decimal.NewFromInt(1000),
		OnHold:    decimal.NewFromInt(500),
		Version:   1,
	}
	amt := mtransaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.RELEASE, Direction: "debit"}

	ctx, recorder := spantest.RecordingContext()

	ops, err := (&UseCase{}).buildDoubleEntryCanceledOps(
		ctx, blc, mtransaction.FromTo{AccountAlias: "@sender", IsFrom: true}, amt, mtransaction.Balance{},
		transaction.Transaction{ID: "txn-1"}, mtransaction.Transaction{Send: mtransaction.Send{Asset: "BRL"}},
		time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), false,
	)
	require.NoError(t, err)
	require.Len(t, ops, 2)

	spantest.AssertNoSpan(t, recorder, removedBuildOperationsSpans...)
	assert.Empty(t, spantest.SpanNames(recorder))
}
