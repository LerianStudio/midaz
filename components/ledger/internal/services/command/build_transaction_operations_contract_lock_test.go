// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type rowContractCase struct {
	Name   string              `json:"name"`
	Status string              `json:"status"`
	Routed bool                `json:"routed"`
	Noted  bool                `json:"noted"`
	Legs   []rowContractLeg    `json:"legs"`
	Want   []rowContractGolden `json:"want"`
}

type rowContractState struct {
	Available string
	OnHold    string
	Used      string
	Version   int64
}

type rowContractGolden struct {
	Type, Alias, Key, Amount, Direction, RouteID string
	Before, After                                rowContractState
	SnapshotBefore, SnapshotAfter                string
	BalanceAffected                              bool
}

type rowContractLeg struct {
	RouteID                                  string
	Alias, Key, Operation, Direction, Amount string
	From                                     bool
	Before, After                            rowContractState
}

func rowContractBalance(leg rowContractLeg, state rowContractState) *mmodel.Balance {
	balance := &mmodel.Balance{
		ID: "11111111-1111-4111-8111-111111111111", AccountID: "22222222-2222-4222-8222-222222222222",
		OrganizationID: "33333333-3333-4333-8333-333333333333", LedgerID: "44444444-4444-4444-8444-444444444444",
		Alias: leg.Alias, Key: leg.Key, AssetCode: "USD", AccountType: "deposit", Direction: constant.DirectionCredit,
		Available: decimal.RequireFromString(state.Available), OnHold: decimal.RequireFromString(state.OnHold),
		OverdraftUsed: decimal.RequireFromString(state.Used), Version: state.Version, AllowSending: true, AllowReceiving: true,
		Settings: &mmodel.BalanceSettings{AllowOverdraft: true},
	}
	if leg.Key == constant.OverdraftBalanceKey {
		balance.Direction = constant.DirectionDebit
	}
	return balance
}

func rowContractObserved(t *testing.T, op *operation.Operation) rowContractGolden {
	t.Helper()
	require.NotNil(t, op.Amount.Value)
	require.NotNil(t, op.Balance.Available)
	require.NotNil(t, op.Balance.OnHold)
	require.NotNil(t, op.Balance.Version)
	require.NotNil(t, op.BalanceAfter.Available)
	require.NotNil(t, op.BalanceAfter.OnHold)
	require.NotNil(t, op.BalanceAfter.Version)
	var route string
	if op.RouteID != nil {
		route = *op.RouteID
	}
	return rowContractGolden{
		Type: op.Type, Alias: op.AccountAlias, Key: op.BalanceKey, Amount: op.Amount.Value.String(), Direction: op.Direction, RouteID: route,
		Before:         rowContractState{op.Balance.Available.String(), op.Balance.OnHold.String(), op.Balance.OverdraftUsed.String(), *op.Balance.Version},
		After:          rowContractState{op.BalanceAfter.Available.String(), op.BalanceAfter.OnHold.String(), op.BalanceAfter.OverdraftUsed.String(), *op.BalanceAfter.Version},
		SnapshotBefore: op.Snapshot.OverdraftUsedBefore, SnapshotAfter: op.Snapshot.OverdraftUsedAfter,
		BalanceAffected: op.BalanceAffected,
	}
}

func loadRowContractCases(t *testing.T) []rowContractCase {
	t.Helper()

	fixtureFile, err := os.Open("testdata/engine_contract/rows.json")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fixtureFile.Close()) })
	decoder := json.NewDecoder(fixtureFile)
	decoder.DisallowUnknownFields()
	var fixture struct {
		SchemaVersion int               `json:"schemaVersion"`
		Cases         []rowContractCase `json:"cases"`
	}
	require.NoError(t, decoder.Decode(&fixture))
	var trailing any
	require.ErrorIs(t, decoder.Decode(&trailing), io.EOF)
	require.Equal(t, 1, fixture.SchemaVersion, "unsupported row fixture schema")
	require.Len(t, fixture.Cases, 13)
	cases := fixture.Cases
	seenNames := make(map[string]bool, len(cases))
	for _, tc := range cases {
		require.NotEmpty(t, tc.Name)
		require.False(t, seenNames[tc.Name], "duplicate scenario name")
		seenNames[tc.Name] = true
		require.NotEmpty(t, tc.Legs)
		require.NotEmpty(t, tc.Want)
	}

	return cases
}

func TestBuildOperations_ContractLock(t *testing.T) {
	const route = "55555555-5555-4555-8555-555555555555"
	cases := loadRowContractCases(t)

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			before := make([]*mmodel.Balance, 0, len(tc.Legs))
			after := make([]*mmodel.Balance, 0, len(tc.Legs))
			entries := make([]mtransaction.FromTo, 0, len(tc.Legs))
			validated := &mtransaction.Responses{From: map[string]mtransaction.Amount{}, To: map[string]mtransaction.Amount{}}
			for _, item := range tc.Legs {
				before = append(before, rowContractBalance(item, item.Before))
				after = append(after, rowContractBalance(item, item.After))
				amount := mtransaction.Amount{Asset: "USD", Value: decimal.RequireFromString(item.Amount), Operation: item.Operation, TransactionType: tc.Status, Direction: item.Direction, RouteValidationEnabled: tc.Routed}
				routeID := route
				if item.RouteID != "" {
					routeID = item.RouteID
				}
				entry := mtransaction.FromTo{AccountAlias: item.Alias, BalanceKey: item.Key, Amount: &amount, IsFrom: item.From, RouteID: &routeID}
				if item.Key != constant.OverdraftBalanceKey {
					entry.Metadata = map[string]any{"purpose": "transfer"}
					entry.ChartOfAccounts = "customer"
				}
				entries = append(entries, entry)
				if item.From {
					validated.From[item.Alias] = amount
				} else {
					validated.To[item.Alias] = amount
				}
			}
			date := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
			tran := transaction.Transaction{ID: "66666666-6666-4666-8666-666666666666", OrganizationID: "33333333-3333-4333-8333-333333333333", LedgerID: "44444444-4444-4444-8444-444444444444"}
			input := mtransaction.Transaction{Send: mtransaction.Send{Asset: "USD"}}
			if tc.Noted {
				after = nil
			}
			rows, _, err := (&UseCase{}).BuildOperations(t.Context(), before, after, entries, input, tran, validated, date, tc.Noted, tc.Routed, nil, "")
			require.NoError(t, err)
			require.Len(t, rows, len(tc.Want))
			for i, row := range rows {
				assert.Equal(t, tc.Want[i], rowContractObserved(t, row), "row %d", i)
				assert.Equal(t, date, row.CreatedAt)
				if row.BalanceKey == constant.OverdraftBalanceKey {
					assert.Empty(t, row.Metadata)
					assert.Empty(t, row.ChartOfAccounts)
				} else {
					assert.Equal(t, map[string]any{"purpose": "transfer"}, row.Metadata)
					assert.Equal(t, "customer", row.ChartOfAccounts)
				}
			}
		})
	}
}
