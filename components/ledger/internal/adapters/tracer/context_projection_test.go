// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func projectionFixture() (ContextInput, tracercontract.Limits) {
	org := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	ledger := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	account := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	asset := uuid.MustParse("550e8400-e29b-41d4-a716-446655440004")
	blocked := false
	return ContextInput{
		Namespace: "ledger-instance", OrganizationID: org, LedgerID: ledger,
		Accounts: []*mmodel.Account{{ID: account.String(), OrganizationID: org.String(), LedgerID: ledger.String(), AssetCode: "BTC", Type: "deposit", Status: mmodel.Status{Code: "ACTIVE"}, Blocked: &blocked}},
		Assets:   []*mmodel.Asset{{ID: asset.String(), OrganizationID: org.String(), LedgerID: ledger.String(), Code: "BTC"}},
		Entries: []PreparedEntry{
			{AccountID: account, Direction: tracercontract.Debit, Amount: decimal.RequireFromString("9007199254740993.00000001"), AssetCode: "BTC"},
			{External: true, Direction: tracercontract.Credit, Amount: decimal.RequireFromString("9007199254740993.00000001"), AssetCode: "BTC"},
		},
	}, tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
}

func TestBuildEvaluationContextOfficialFacts(t *testing.T) {
	t.Parallel()
	input, limits := projectionFixture()
	result, err := BuildEvaluationContext(context.Background(), input, limits)
	require.NoError(t, err)
	require.Len(t, result.Accounts, 1)
	require.Len(t, result.Entries, 2)
	require.Equal(t, "deposit", result.Accounts[0].Type)
	require.Equal(t, "ACTIVE", result.Accounts[0].Status)
	require.NotNil(t, result.Accounts[0].Blocked)
	require.False(t, *result.Accounts[0].Blocked)
	require.Equal(t, input.Assets[0].ID, result.Entries[0].Asset.ID)
	require.Equal(t, tracercontract.Amount("9007199254740993.00000001"), result.Entries[0].Amount)
	require.True(t, result.Entries[1].External)
	require.Equal(t, uuid.Nil, result.Entries[1].AccountID)
	// Freeze the projected facts; later caller mutations cannot change a request in flight.
	*input.Accounts[0].Blocked = true
	require.False(t, *result.Accounts[0].Blocked)
}

func TestBuildEvaluationContextRejectsAmbiguousOrUntrustedFacts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*ContextInput)
	}{
		{name: "account from another organization", change: func(i *ContextInput) { i.Accounts[0].OrganizationID = i.LedgerID.String() }},
		{name: "asset from another ledger", change: func(i *ContextInput) { i.Assets[0].LedgerID = i.OrganizationID.String() }},
		{name: "invalid asset UUID", change: func(i *ContextInput) { i.Assets[0].ID = "not-a-uuid" }},
		{name: "missing account blocked", change: func(i *ContextInput) { i.Accounts[0].Blocked = nil }},
		{name: "unresolved code", change: func(i *ContextInput) { i.Entries[0].AssetCode = "BRL" }},
		{name: "duplicate code", change: func(i *ContextInput) {
			other := *i.Assets[0]
			other.ID = i.OrganizationID.String()
			i.Assets = append(i.Assets, &other)
		}},
		{name: "duplicate asset id with different codes", change: func(i *ContextInput) { other := *i.Assets[0]; other.Code = "USD"; i.Assets = append(i.Assets, &other) }},
		{name: "nil asset", change: func(i *ContextInput) { i.Assets[0] = nil }},
		{name: "nil account", change: func(i *ContextInput) { i.Accounts[0] = nil }},
		{name: "external claims account", change: func(i *ContextInput) { i.Entries[0].External = true }},
		{name: "extreme decimal exponent", change: func(i *ContextInput) { i.Entries[0].Amount = decimal.New(1, 1000000000) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input, limits := projectionFixture()
			tt.change(&input)
			_, err := BuildEvaluationContext(context.Background(), input, limits)
			require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		})
	}
}

func TestBuildEvaluationContextCancellation(t *testing.T) {
	t.Parallel()
	input, limits := projectionFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := BuildEvaluationContext(ctx, input, limits)
	require.ErrorIs(t, err, context.Canceled)
}
