// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestBuildTransactionEvidenceViewsSeparatesPublicAndDurableStatus(t *testing.T) {
	plan, result := recoveryContractFixture(t)
	plan.TransactionStatus = constant.CREATED
	plan.IntentFingerprint = mustEvidenceViewFingerprint(t, plan)
	record := recoveryContractEnvelope(t, plan, result)

	views, err := BuildTransactionEvidenceViews(record)
	require.NoError(t, err)
	require.NotNil(t, views.InitialResponse)
	require.NotNil(t, views.Lookup)
	require.NotNil(t, views.Projection.Transaction)
	assert.Equal(t, constant.CREATED, views.InitialResponse.Status.Code)
	assert.Equal(t, constant.APPROVED, views.Lookup.Status.Code)
	assert.Equal(t, constant.APPROVED, views.Projection.Transaction.Status.Code)

	views.InitialResponse.Operations[0].Metadata["purpose"] = "changed"
	*views.InitialResponse.Operations[0].RouteID = "changed"
	assert.Equal(t, "transfer", views.Lookup.Operations[0].Metadata["purpose"])
	assert.NotEqual(t, "changed", *views.Lookup.Operations[0].RouteID)
	assert.Equal(t, "transfer", views.Projection.Transaction.Operations[0].Metadata["purpose"])
}

func TestBuildTransactionEvidenceViewsPreservesPendingAsNonTerminal(t *testing.T) {
	plan, result := recoveryContractFixture(t)
	plan.TransactionStatus = constant.PENDING
	plan.Action = constant.ActionHold
	plan.TransactionInput.Pending = true
	plan.IntentFingerprint = mustEvidenceViewFingerprint(t, plan)
	record := recoveryContractEnvelope(t, plan, result)

	views, err := BuildTransactionEvidenceViews(record)
	require.NoError(t, err)
	assert.Equal(t, constant.PENDING, views.InitialResponse.Status.Code)
	assert.Equal(t, constant.PENDING, views.Lookup.Status.Code)
	assert.Equal(t, constant.PENDING, views.Projection.Transaction.Status.Code)
	assert.False(t, views.Lookup.Body.IsEmpty())
}

func TestBuildTransactionEvidenceViewsPreservesDistinctFeeRouteLegs(t *testing.T) {
	plan, result := recoveryContractFixture(t)
	primary := plan.OperationSpecs[0]
	fee := primary
	fee.PostingRef = "source:fee:0"
	fee.BalanceRef = "@source#fees"
	fee.Balance.ID = uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa").String()
	fee.Balance.Key = "fees"
	fee.RequestedAmount = decimal.NewFromInt(5)
	feeRoute := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	fee.RouteID = &feeRoute
	fee.Metadata = map[string]any{constant.MetadataKeyFeeLeg: constant.MetadataValueFeeLeg}
	plan.OperationSpecs = append(plan.OperationSpecs, fee)
	plan.IntentFingerprint = mustEvidenceViewFingerprint(t, plan)

	movement := result.Movements[0]
	movement.Ref = "movement:fee:0"
	movement.PostingRef = fee.PostingRef
	movement.BalanceRef = fee.BalanceRef
	movement.Amount = decimal.NewFromInt(5)
	result.Movements = append(result.Movements, movement)
	result.Final = append(result.Final, accounting.BalanceSnapshot{
		BalanceRef:  fee.BalanceRef,
		ID:          uuid.MustParse(fee.Balance.ID),
		AccountID:   uuid.MustParse(fee.Balance.AccountID),
		AccountType: fee.Balance.AccountType,
		AssetCode:   fee.Balance.AssetCode,
		Alias:       fee.Balance.Alias,
		Key:         fee.Balance.Key,
		Direction:   fee.Balance.Direction,
		Available:   movement.After.Available,
		OnHold:      movement.After.OnHold,
		Version:     movement.After.Version,
	})

	record := recoveryContractEnvelope(t, plan, result)
	views, err := BuildTransactionEvidenceViews(record)
	require.NoError(t, err)
	require.Len(t, views.Lookup.Operations, 2)
	assert.Equal(t, "default", views.Lookup.Operations[0].BalanceKey)
	assert.Equal(t, "fees", views.Lookup.Operations[1].BalanceKey)
	assert.NotEqual(t, *views.Lookup.Operations[0].RouteID, *views.Lookup.Operations[1].RouteID)
	assert.NotContains(t, views.Lookup.Operations[0].Metadata, constant.MetadataKeyFeeLeg)
	assert.Equal(t, constant.MetadataValueFeeLeg, views.Lookup.Operations[1].Metadata[constant.MetadataKeyFeeLeg])
}

func mustEvidenceViewFingerprint(t testing.TB, plan TransactionCompletionPlan) string {
	t.Helper()

	fingerprint, err := ComputeEngineIntentFingerprint(recoveryContractIntent(plan))
	require.NoError(t, err)

	return fingerprint
}
