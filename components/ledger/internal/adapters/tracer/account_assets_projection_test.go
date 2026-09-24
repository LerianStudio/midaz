// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/require"
)

func TestBuildAccountAssetsOfficialRecords(t *testing.T) {
	input, bounds := projectionFixture()
	input.Accounts[0].Blocked = nil // Asset facts do not require policy attributes.
	facts, err := BuildAccountAssets(t.Context(), AccountAssetsInput{Namespace: input.Namespace, OrganizationID: input.OrganizationID, LedgerID: input.LedgerID, Accounts: input.Accounts, Assets: input.Assets}, bounds)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, input.Accounts[0].ID, facts[0].AccountID.String())
	require.Equal(t, input.Assets[0].ID, facts[0].Asset.ID)
	require.Equal(t, input.Namespace, facts[0].Asset.Namespace)
	input.Assets[0].Code = "changed"
	require.Equal(t, "BTC", facts[0].Asset.Code)
}

func TestBuildAccountAssetsRejectsMissingOrAmbiguousRecords(t *testing.T) {
	for _, change := range []func(*ContextInput){
		func(i *ContextInput) { i.Accounts[0].OrganizationID = i.LedgerID.String() },
		func(i *ContextInput) { i.Assets[0].LedgerID = i.OrganizationID.String() },
		func(i *ContextInput) { i.Assets = nil }, func(i *ContextInput) { i.Accounts = append(i.Accounts, i.Accounts[0]) },
		func(i *ContextInput) {
			copy := *i.Assets[0]
			copy.ID = i.OrganizationID.String()
			i.Assets = append(i.Assets, &copy)
		},
		func(i *ContextInput) { i.Accounts = []*mmodel.Account{nil} },
	} {
		input, bounds := projectionFixture()
		change(&input)
		facts, err := BuildAccountAssets(t.Context(), AccountAssetsInput{Namespace: input.Namespace, OrganizationID: input.OrganizationID, LedgerID: input.LedgerID, Accounts: input.Accounts, Assets: input.Assets}, bounds)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		require.Nil(t, facts)
	}
}
