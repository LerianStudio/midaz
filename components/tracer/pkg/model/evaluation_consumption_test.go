// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func consumptionFixture() (tracercontract.Context, tracercontract.Limits) {
	blocked := false
	a := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	b := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	asset := tracercontract.AssetRef{Namespace: "ledger", ID: "btc-1", Code: "BTC"}
	return tracercontract.Context{
		Accounts: []tracercontract.Account{
			{ID: a, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset},
			{ID: b, Type: "fee-payer", Status: "ACTIVE", Blocked: &blocked, Asset: asset},
		},
		Entries: []tracercontract.Entry{
			{AccountID: b, Direction: tracercontract.Debit, Amount: "0.00000001", Asset: asset},
			{AccountID: a, Direction: tracercontract.Debit, Amount: "9007199254740993", Asset: asset},
			{AccountID: a, Direction: tracercontract.Debit, Amount: "0.00000001", Asset: asset},
			{AccountID: a, Direction: tracercontract.Credit, Amount: "100", Asset: asset},
			{External: true, Direction: tracercontract.Debit, Amount: "20", Asset: asset},
		},
	}, tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 100, MaxIntegerDigits: 128, MaxFractionDigits: 128}
}

func TestAccountDebitsGrossExactAndOrdered(t *testing.T) {
	t.Parallel()
	c, limits := consumptionFixture()
	totals, err := AccountDebits(context.Background(), c, "ledger", limits)
	require.NoError(t, err)
	require.Len(t, totals, 2)
	require.Equal(t, c.Accounts[0].ID, totals[0].AccountID)
	require.Equal(t, "9007199254740993.00000001", totals[0].Amount.String())
	require.Equal(t, c.Accounts[1].ID, totals[1].AccountID)
	require.Equal(t, "0.00000001", totals[1].Amount.String())
	require.Equal(t, "0.00000001", string(c.Entries[0].Amount))
	// Reverse input order to represent two callers approaching the same accounts
	// in opposite orders. Counter/lock orchestration receives the same order.
	for i, j := 0, len(c.Entries)-1; i < j; i, j = i+1, j-1 {
		c.Entries[i], c.Entries[j] = c.Entries[j], c.Entries[i]
	}
	again, err := AccountDebits(context.Background(), c, "ledger", limits)
	require.NoError(t, err)
	require.Equal(t, totals, again)
}

func TestAccountDebitsSeparateAssetIdentities(t *testing.T) {
	t.Parallel()
	c, limits := consumptionFixture()
	c.Accounts[1].Asset.ID = "btc-2"
	c.Entries[0].Asset = c.Accounts[1].Asset
	totals, err := AccountDebits(context.Background(), c, "ledger", limits)
	require.NoError(t, err)
	require.Len(t, totals, 2)
	require.NotEqual(t, totals[0].Asset.Identity(), totals[1].Asset.Identity())
}

func TestAccountDebitsNoCreditOffsetOrExternalCounter(t *testing.T) {
	t.Parallel()
	c, limits := consumptionFixture()
	for i := range c.Entries {
		c.Entries[i].Direction = tracercontract.Credit
	}
	totals, err := AccountDebits(context.Background(), c, "ledger", limits)
	require.NoError(t, err)
	require.Empty(t, totals)

	c.Accounts = nil
	c.Entries = c.Entries[len(c.Entries)-1:]
	c.Entries[0].Direction = tracercontract.Debit
	totals, err = AccountDebits(context.Background(), c, "ledger", limits)
	require.NoError(t, err)
	require.Empty(t, totals)
}

func TestAccountDebitsRejectsIncompleteContextAndOverflow(t *testing.T) {
	t.Parallel()
	c, limits := consumptionFixture()
	c.Accounts[0].Blocked = nil
	_, err := AccountDebits(context.Background(), c, "ledger", limits)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)

	c, limits = consumptionFixture()
	limits.MaxIntegerDigits = 2
	c.Entries[1].Amount = "99"
	c.Entries[2].Amount = "1"
	c.Entries[3].Amount = "1"
	_, err = AccountDebits(context.Background(), c, "ledger", limits)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = AccountDebits(ctx, c, "ledger", limits)
	require.ErrorIs(t, err, context.Canceled)
}
