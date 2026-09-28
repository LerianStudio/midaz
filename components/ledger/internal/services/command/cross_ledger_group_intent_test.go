// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestBuildCrossLedgerGroupIntent_ClassifiesNetPartsAndPreservesOrder(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("0199a200-0000-7000-8000-000000000001")
	ledgerA := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a200-0000-7000-8000-000000000002")}
	ledgerB := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a200-0000-7000-8000-000000000003")}
	transaction := crossLedgerTestTransaction("100",
		[]mtransaction.FromTo{crossLedgerAmountLeg("@a-debit", "100", true)},
		[]mtransaction.FromTo{
			crossLedgerAmountLeg("@a-credit", "30", false),
			crossLedgerAmountLeg("@b-credit", "70", false),
		})

	parts, err := decomposeCrossLedgerTransaction(transaction, crossLedgerTransactionScopes{
		from: []atomicTransactionBatchLedgerRef{ledgerA},
		to:   []atomicTransactionBatchLedgerRef{ledgerA, ledgerB},
	})
	require.NoError(t, err)

	intent, err := buildCrossLedgerGroupIntent(transaction.Send.Asset, parts)
	require.NoError(t, err)
	require.Len(t, intent.Parts, 2)
	assert.Equal(t, CrossLedgerGroupIntentFormatVersion, intent.FormatVersion)
	assert.Equal(t, "BRL", intent.Asset)
	assert.Equal(t, CrossLedgerGroupRoleOrigin, intent.Parts[0].Role)
	assert.Equal(t, 1, intent.Parts[0].Order)
	assert.Equal(t, ledgerA.organizationID, intent.Parts[0].OrganizationID)
	assert.Equal(t, ledgerA.ledgerID, intent.Parts[0].LedgerID)
	assert.Equal(t, CrossLedgerGroupRoleDestination, intent.Parts[1].Role)
	assert.Equal(t, 2, intent.Parts[1].Order)
	assert.Equal(t, ledgerB.ledgerID, intent.Parts[1].LedgerID)
}

func TestCrossLedgerGroupIntent_JSONShapeAndRoundTrip(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("0199a200-0000-7000-8000-000000000011")
	ledgerA := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a200-0000-7000-8000-000000000012")}
	ledgerB := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a200-0000-7000-8000-000000000013")}
	transaction := crossLedgerTestTransaction("100",
		[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
		[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)})
	parts, err := decomposeCrossLedgerTransaction(transaction, crossLedgerTransactionScopes{
		from: []atomicTransactionBatchLedgerRef{ledgerA},
		to:   []atomicTransactionBatchLedgerRef{ledgerB},
	})
	require.NoError(t, err)

	intent, err := buildCrossLedgerGroupIntent("BRL", parts)
	require.NoError(t, err)
	raw, err := encodeCrossLedgerGroupIntent(intent)
	require.NoError(t, err)

	var shape map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &shape))
	assert.ElementsMatch(t, []string{"formatVersion", "asset", "parts"}, mapKeys(shape))
	assert.JSONEq(t, `1`, string(shape["formatVersion"]))
	assert.JSONEq(t, `"BRL"`, string(shape["asset"]))

	decoded, err := decodeCrossLedgerGroupIntent(raw)
	require.NoError(t, err)
	roundTrip, err := encodeCrossLedgerGroupIntent(*decoded)
	require.NoError(t, err)
	assert.JSONEq(t, string(raw), string(roundTrip))
	assert.Equal(t, intent.Parts[0].OrganizationID, decoded.Parts[0].OrganizationID)
	assert.Equal(t, intent.Parts[1].LedgerID, decoded.Parts[1].LedgerID)
	assert.Nil(t, decoded.Parts[0].Transaction.Send.Source.From[0].Share)
	assert.Empty(t, decoded.Parts[0].Transaction.Send.Source.From[0].Remaining)
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	return keys
}
