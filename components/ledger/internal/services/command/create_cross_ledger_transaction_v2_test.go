// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestBuildCrossLedgerAtomicBatchInput_UsesOneGroupAndOrderedParts(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000301")
	ledgerA := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("01994f13-29b7-7000-8000-000000000302")}
	ledgerB := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("01994f13-29b7-7000-8000-000000000303")}
	groupID := uuid.MustParse("01994f13-29b7-7000-8000-000000000304")
	ttl := 10 * time.Minute
	tx := crossLedgerTestTransaction("100",
		[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
		[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)})

	parts, err := decomposeCrossLedgerTransaction(tx, crossLedgerTransactionScopes{
		from: []atomicTransactionBatchLedgerRef{ledgerA},
		to:   []atomicTransactionBatchLedgerRef{ledgerB},
	})
	require.NoError(t, err)

	got := buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{
		Transaction: tx,
		Scopes: CrossLedgerTransactionScopes{
			Debits:  []CrossLedgerLegScope{{OrganizationID: ledgerA.organizationID, LedgerID: ledgerA.ledgerID}},
			Credits: []CrossLedgerLegScope{{OrganizationID: ledgerB.organizationID, LedgerID: ledgerB.ledgerID}},
		},
		IdempotencyKey:     "request-key",
		IdempotencyTTL:     ttl,
		CanonicalRequest:   []byte(`{"amount":"100"}`),
		RequestFingerprint: "fingerprint",
	}, groupID, parts)

	require.NotNil(t, got.GroupID)
	assert.Equal(t, groupID, *got.GroupID)
	assert.True(t, got.CrossLedgerGroup)
	assert.Equal(t, "request-key", got.IdempotencyKey)
	assert.Equal(t, ttl, got.IdempotencyTTL)
	assert.Equal(t, []byte(`{"amount":"100"}`), got.CanonicalRequest)
	assert.Equal(t, "fingerprint", got.RequestFingerprint)
	require.Len(t, got.Transactions, 2)
	assert.Equal(t, ledgerA.organizationID, got.Transactions[0].OrganizationID)
	assert.Equal(t, ledgerA.ledgerID, got.Transactions[0].LedgerID)
	assert.Equal(t, constant.ActionDirect, got.Transactions[0].Action)
	assert.Equal(t, 1, got.Transactions[0].Order)
	assert.Equal(t, ledgerB.ledgerID, got.Transactions[1].LedgerID)
	assert.Equal(t, 2, got.Transactions[1].Order)
	assert.Equal(t, "@external/BRL", got.Transactions[0].Transaction.Send.Distribute.To[0].AccountAlias)
	assert.Equal(t, "@external/BRL", got.Transactions[1].Transaction.Send.Source.From[0].AccountAlias)
}
