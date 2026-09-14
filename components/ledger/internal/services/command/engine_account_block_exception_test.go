// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestBindEngineAccountBlockException_SelectsExactlyOneActionOutflow(t *testing.T) {
	t.Parallel()

	grant := &mtransaction.AccountBlockExceptionGrant{
		ID: uuid.MustParse("6e0ebc70-6039-4edf-b039-4bb5d85afafe"), Alias: "@source", Amount: "999",
	}
	balances := map[string]*mmodel.Balance{
		"@source#default":   {Alias: "@source", Key: "default"},
		"@source#segment":   {Alias: "@source", Key: "segment"},
		"@source#overdraft": {Alias: "@source", Key: constant.OverdraftBalanceKey},
		"@target#default":   {Alias: "@target", Key: "default"},
	}

	for _, test := range []struct {
		name        string
		action      string
		postings    []accounting.Posting
		wantRef     string
		wantAmount  string
		wantCode    string
		wantInvalid bool
	}{
		{
			name: "direct debit", action: constant.ActionDirect, wantRef: "source", wantAmount: "30",
			postings: []accounting.Posting{{Ref: "source", BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(30)}},
		},
		{
			name: "revert debit", action: constant.ActionRevert, wantRef: "source", wantAmount: "31",
			postings: []accounting.Posting{{Ref: "source", BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(31)}},
		},
		{
			name: "commit unreserve", action: constant.ActionCommit, wantRef: "source", wantAmount: "32",
			postings: []accounting.Posting{{Ref: "source", BalanceRef: "@source#default", Type: accounting.PostingUnreserve, Amount: decimal.NewFromInt(32)}},
		},
		{
			name: "no matching outflow", action: constant.ActionDirect, wantCode: constant.ErrAccountBlockExceptionInvalid.Error(),
			postings: []accounting.Posting{{Ref: "target", BalanceRef: "@source#default", Type: accounting.PostingCredit, Amount: decimal.NewFromInt(30)}},
		},
		{
			name: "two matching outflows", action: constant.ActionDirect, wantCode: constant.ErrAccountBlockExceptionInvalid.Error(),
			postings: []accounting.Posting{
				{Ref: "first", BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(10)},
				{Ref: "second", BalanceRef: "@source#segment", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(20)},
			},
		},
		{
			name: "alias only credited", action: constant.ActionDirect, wantCode: constant.ErrAccountBlockExceptionInvalid.Error(),
			postings: []accounting.Posting{
				{Ref: "other", BalanceRef: "@target#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(30)},
				{Ref: "source", BalanceRef: "@source#default", Type: accounting.PostingCredit, Amount: decimal.NewFromInt(30)},
			},
		},
		{
			name: "overdraft balance is not primary", action: constant.ActionDirect, wantCode: constant.ErrAccountBlockExceptionInvalid.Error(),
			postings: []accounting.Posting{{Ref: "overdraft", BalanceRef: "@source#overdraft", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(30)}},
		},
		{
			name: "hold action", action: constant.ActionHold, wantInvalid: true,
			postings: []accounting.Posting{{Ref: "source", BalanceRef: "@source#default", Type: accounting.PostingHold, Amount: decimal.NewFromInt(30)}},
		},
		{
			name: "cancel action", action: constant.ActionCancel, wantInvalid: true,
			postings: []accounting.Posting{{Ref: "source", BalanceRef: "@source#default", Type: accounting.PostingRelease, Amount: decimal.NewFromInt(30)}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			exception, err := bindEngineAccountBlockException(test.action, grant, test.postings, balances)
			if test.wantCode != "" {
				require.ErrorContains(t, err, test.wantCode)
				require.Nil(t, exception)
				return
			}
			if test.wantInvalid {
				require.ErrorIs(t, err, ErrInvalidEngineTranslation)
				require.Nil(t, exception)
				return
			}

			require.NoError(t, err)
			require.Equal(t, grant.ID, exception.ExceptionID)
			require.Equal(t, grant.Alias, exception.Alias)
			require.Equal(t, test.wantRef, exception.PrimaryPostingRef)
			require.Equal(t, test.wantAmount, exception.Amount.String())
		})
	}
}

func TestBindEngineAccountBlockException_NilGrantDoesNotConstrainTheAction(t *testing.T) {
	t.Parallel()

	exception, err := bindEngineAccountBlockException("future", nil, nil, nil)
	require.NoError(t, err)
	require.Nil(t, exception)
}
