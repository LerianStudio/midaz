//go:build unit

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasonForOperation(t *testing.T) {
	cases := []struct {
		name     string
		opType   string
		expected string
	}{
		{"credit", "CREDIT", events.BalanceChangeReasonCredit},
		{"debit", "DEBIT", events.BalanceChangeReasonDebit},
		{"block", "BLOCK", events.BalanceChangeReasonBlock},
		{"unblock", "UNBLOCK", events.BalanceChangeReasonUnblock},
		{"hold", "ON_HOLD", events.BalanceChangeReasonHold},
		{"release", "RELEASE", events.BalanceChangeReasonRelease},
		{"overdraft", "OVERDRAFT", events.BalanceChangeReasonOverdraft},
		{"unknown falls back to adjust", "SOMETHING_NEW", events.BalanceChangeReasonAdjust},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, reasonForOperation(tc.opType))
		})
	}
}

func TestSendBalanceChangedEvents_EmitsPerBalanceAffectingOperation(t *testing.T) {
	// GIVEN a recording emitter capturing EmitRequests.
	rec := pkgStreaming.NewMockEmitter()
	uc := &UseCase{Streaming: rec}

	avail := decimal.NewFromInt(1500)
	onHold := decimal.NewFromInt(0)
	amt := decimal.NewFromInt(100)
	ver := int64(7)

	avail2 := decimal.NewFromInt(900)
	onHold2 := decimal.NewFromInt(0)
	amt2 := decimal.NewFromInt(600)
	ver2 := int64(8)

	tran := &transaction.Transaction{
		ID:             "txn-1",
		OrganizationID: "org-1",
		LedgerID:       "led-1",
		Operations: []*operation.Operation{
			{
				ID:              "op-1",
				AccountID:       "acc-1",
				BalanceID:       "bal-1",
				AccountAlias:    "@person1",
				AssetCode:       "BRL",
				BalanceKey:      "default",
				Type:            "CREDIT",
				Direction:       "credit",
				BalanceAffected: true,
				Amount:          operation.Amount{Value: &amt},
				BalanceAfter:    operation.Balance{Available: &avail, OnHold: &onHold, Version: &ver},
			},
			{
				// Second balance-affecting op — proves one event PER operation.
				ID:              "op-2",
				AccountID:       "acc-2",
				BalanceID:       "bal-2",
				AccountAlias:    "@person2",
				AssetCode:       "BRL",
				BalanceKey:      "default",
				Type:            "DEBIT",
				Direction:       "debit",
				BalanceAffected: true,
				Amount:          operation.Amount{Value: &amt2},
				BalanceAfter:    operation.Balance{Available: &avail2, OnHold: &onHold2, Version: &ver2},
			},
			{
				ID:              "op-3",
				BalanceAffected: false, // must not emit
				AccountID:       "acc-3",
				Type:            "DEBIT",
			},
		},
	}

	uc.SendBalanceChangedEvents(context.Background(), tran)

	// M1: exactly one event per balance-affecting op, two DISTINCT subjects.
	reqs := rec.Events()
	require.Len(t, reqs, 2)
	assert.Equal(t, "balance.changed", reqs[0].DefinitionKey)
	assert.Equal(t, "balance.changed", reqs[1].DefinitionKey)
	assert.Equal(t, "txn-1:op-1", reqs[0].Subject)
	assert.Equal(t, "txn-1:op-2", reqs[1].Subject)

	// H1 + M2: decode the first op's wire payload and assert the full
	// operation -> payload mapping so a field-swap regression fails here.
	var payload events.BalanceChangedPayload
	require.NoError(t, json.Unmarshal(reqs[0].Payload, &payload))

	assert.Equal(t, "acc-1", payload.AccountID)
	assert.Equal(t, "bal-1", payload.BalanceID)
	assert.Equal(t, "BRL", payload.AssetCode)
	assert.Equal(t, "default", payload.BalanceKey)
	assert.Equal(t, "@person1", payload.Alias)
	assert.Equal(t, events.BalanceChangeReasonCredit, payload.Reason)
	assert.Equal(t, "CREDIT", payload.OperationType)
	assert.Equal(t, "credit", payload.Direction)
	assert.True(t, decimal.NewFromInt(1500).Equal(payload.Available), "available: got %s", payload.Available)
	assert.True(t, decimal.NewFromInt(0).Equal(payload.OnHold), "onHold: got %s", payload.OnHold)
	assert.True(t, decimal.NewFromInt(100).Equal(payload.Amount), "amount: got %s", payload.Amount)
	assert.Equal(t, int64(7), payload.Version)
}

func TestSendBalanceChangedEvents_NilSafe(t *testing.T) {
	uc := &UseCase{Streaming: nil}
	// nil Streaming → no emit / no panic.
	uc.SendBalanceChangedEvents(context.Background(), nil)
	uc.SendBalanceChangedEvents(context.Background(), &transaction.Transaction{ID: "txn-1"})

	// A nil operation / nil balance pointers inside a live transaction must
	// also be nil-safe once Streaming is present.
	rec := pkgStreaming.NewMockEmitter()
	uc2 := &UseCase{Streaming: rec}
	uc2.SendBalanceChangedEvents(context.Background(), &transaction.Transaction{
		ID: "txn-2",
		Operations: []*operation.Operation{
			nil,
			{ID: "op-1", Type: "CREDIT", BalanceAffected: true}, // nil Amount/Balance pointers
		},
	})
	// nil op skipped; the second op has nil Amount.Value + nil BalanceAfter
	// pointers but still emits (values default to zero) without panicking.
	reqs := rec.Events()
	require.Len(t, reqs, 1)

	// The nil->zero conversions (decimalOrZero / int64OrZero) must reach the
	// wire as decimal.Zero / version 0, not as absent or garbage values.
	var payload events.BalanceChangedPayload
	require.NoError(t, json.Unmarshal(reqs[0].Payload, &payload))
	assert.True(t, payload.Available.Equal(decimal.Zero), "available: got %s", payload.Available)
	assert.True(t, payload.OnHold.Equal(decimal.Zero), "onHold: got %s", payload.OnHold)
	assert.True(t, payload.Amount.Equal(decimal.Zero), "amount: got %s", payload.Amount)
	assert.Equal(t, int64(0), payload.Version)
}
