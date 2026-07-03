//go:build unit

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendBalanceChangedEvents_PayloadRoundTrip drives one balance-affecting
// BLOCK operation through the emit path and round-trips the recorded wire
// payload back into BalanceChangedPayload. It deliberately uses a BLOCK op with
// Direction "debit": the classifier maps Type "BLOCK" -> reason "block"
// regardless of direction, so this locks reason != direction (a wire-contract
// property the CREDIT/DEBIT cases in send_balance_changed_events_test.go cannot
// observe, since there reason and direction coincide).
func TestSendBalanceChangedEvents_PayloadRoundTrip(t *testing.T) {
	rec := pkgStreaming.NewMockEmitter()
	uc := &UseCase{Streaming: rec}

	avail := decimal.NewFromInt(900)
	onHold := decimal.NewFromInt(100)
	amt := decimal.NewFromInt(50)
	ver := int64(3)

	tran := &transaction.Transaction{
		ID:             "txn-9",
		OrganizationID: "org-9",
		LedgerID:       "led-9",
		Operations: []*operation.Operation{
			{
				ID:              "op-9",
				AccountID:       "acc-9",
				BalanceID:       "bal-9",
				AccountAlias:    "@a9",
				AssetCode:       "BRL",
				BalanceKey:      "default",
				Type:            "BLOCK",
				Direction:       "debit",
				BalanceAffected: true,
				Amount:          operation.Amount{Value: &amt},
				BalanceAfter:    operation.Balance{Available: &avail, OnHold: &onHold, Version: &ver},
			},
		},
	}

	uc.SendBalanceChangedEvents(context.Background(), tran)

	reqs := rec.Events()
	require.Len(t, reqs, 1)
	assert.Equal(t, "balance.changed", reqs[0].DefinitionKey)
	assert.Equal(t, "txn-9:op-9", reqs[0].Subject)

	var p events.BalanceChangedPayload
	require.NoError(t, json.Unmarshal(reqs[0].Payload, &p))

	assert.Equal(t, "acc-9", p.AccountID)
	// reason is driven by Operation.Type, NOT by Direction: BLOCK -> "block"
	// even though Direction is "debit". This locks reason != direction.
	assert.Equal(t, events.BalanceChangeReasonBlock, p.Reason)
	assert.Equal(t, "BLOCK", p.OperationType)
	assert.Equal(t, "debit", p.Direction)
	assert.True(t, decimal.NewFromInt(900).Equal(p.Available), "available: got %s", p.Available)
	assert.Equal(t, int64(3), p.Version)
}
