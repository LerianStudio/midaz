// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

func TestAccountClosingWireDeclaresOneProtectionTripletPerAccount(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()

	second := input.Execution.Balances[0]
	second.ID = uuid.MustParse("2e2f3d0e-8a3e-4f0a-8a2f-51a0b4d1c001")
	second.AccountID = uuid.MustParse("0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d")
	second.Alias, second.BalanceRef = "@target", "@target#default"
	second.Available = decimal.NewFromInt(5)

	// A third balance of the FIRST account: the protection is per account, so it
	// must not add a second triplet.
	third := input.Execution.Balances[0]
	third.ID = uuid.MustParse("3e2f3d0e-8a3e-4f0a-8a2f-51a0b4d1c002")
	third.Alias, third.BalanceRef = "@source", "@source#savings"
	third.Key = "savings"
	third.Available = decimal.NewFromInt(7)

	input.Execution.Balances = append(input.Execution.Balances, second, third)
	for _, balance := range input.Execution.Balances[1:] {
		resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys(
			"tenant:fixture:balance:{transactions}:" + input.Execution.OrganizationID.String() + ":" +
				input.Execution.LedgerID.String() + ":" + balance.BalanceRef)
	}

	resolved.Accounts = testResolvedAccountKeys("tenant:fixture:", input.Execution, testAdmissionToken)

	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)

	accounts := protectedAccounts(input.Execution)
	require.Len(t, accounts, 2, "three balances of two accounts declare two accounts")
	require.Len(t, prepared.Keys, 5+3*len(input.Execution.Balances)+3*len(accounts))

	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Len(t, wire.Accounts, 2)

	for i, account := range wire.Accounts {
		base := 5 + 3*len(input.Execution.Balances) + 3*i
		require.Equal(t, accounts[i].String(), account.AccountID)
		require.Equal(t, base+1, account.ClosingKeyIndex)
		require.Equal(t, base+2, account.ClosedKeyIndex)
		require.Equal(t, base+3, account.OwnershipKeyIndex)
		require.Equal(t, resolved.Accounts[accounts[i]].Closing, prepared.Keys[base])
		require.Equal(t, resolved.Accounts[accounts[i]].Closed, prepared.Keys[base+1])
		require.Equal(t, resolved.Accounts[accounts[i]].Ownership, prepared.Keys[base+2])
		require.Contains(t, account.AdmissionToken, testAdmissionToken)
	}

	require.Less(t, wire.Accounts[0].AccountID, wire.Accounts[1].AccountID, "the block is ordered by account")
}

func TestAccountClosingWireIsolatesEveryAccountAndScope(t *testing.T) {
	t.Parallel()

	first, _, firstResolved := validWireExecution()

	second, _, _ := validWireExecution()
	second.Execution.OrganizationID = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	secondResolved := testResolvedAccountKeys("tenant:fixture:", second.Execution, testAdmissionToken)

	accountID := first.Execution.Balances[0].AccountID
	require.Equal(t, accountID, second.Execution.Balances[0].AccountID)

	require.NotEqual(t, firstResolved.Accounts[accountID], secondResolved[accountID],
		"the same account under another organization resolves to other controls")

	for _, key := range []string{
		firstResolved.Accounts[accountID].Closing,
		firstResolved.Accounts[accountID].Closed,
		firstResolved.Accounts[accountID].Ownership,
	} {
		require.Contains(t, key, first.Execution.OrganizationID.String())
		require.Contains(t, key, first.Execution.LedgerID.String())
		require.Contains(t, key, accountID.String())
	}
}

func TestAccountClosingWireRejectsAMismatchedProtectionInventory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*resolvedExecutionKeys, accounting.Execution)
	}{
		{name: "missing account", mutate: func(resolved *resolvedExecutionKeys, _ accounting.Execution) {
			resolved.Accounts = map[uuid.UUID]resolvedAccountKeys{}
		}},
		{name: "foreign account key", mutate: func(resolved *resolvedExecutionKeys, request accounting.Execution) {
			protection := resolved.Accounts[request.Balances[0].AccountID]
			protection.Ownership = "tenant:fixture:account-admin-ownership:{transactions}:other"
			resolved.Accounts[request.Balances[0].AccountID] = protection
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input, limits, resolved := validWireExecution()
			test.mutate(&resolved, input.Execution)

			prepared, err := prepareExecution(context.Background(), input, limits, resolved)
			require.Error(t, err)
			require.Nil(t, prepared)
		})
	}
}

func TestAccountClosingWireKeepsTheAdmissionTokenOutOfTheKeys(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()

	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)

	for _, key := range prepared.Keys {
		require.NotContains(t, key, testAdmissionToken, "the administrative token is not part of any key")
	}

	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Len(t, wire.Accounts, 1)
	require.Equal(t, testAdmissionToken, wire.Accounts[0].AdmissionToken)
}

func TestAccountClosingWireDeclaresAnEmptyTokenWithoutAnAdmission(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	resolved.Accounts = testResolvedAccountKeys("tenant:fixture:", input.Execution, "")

	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)

	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Len(t, wire.Accounts, 1)
	require.Empty(t, wire.Accounts[0].AdmissionToken)
}
