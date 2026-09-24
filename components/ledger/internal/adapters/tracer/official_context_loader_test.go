// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer/mocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestOfficialContextLoader(t *testing.T) {
	for _, scenario := range []string{"context", "binding", "read error", "invalid entry", "extra account"} {
		t.Run(scenario, func(t *testing.T) {
			input, bounds := projectionFixture()
			reader := mocks.NewMockOfficialRecordsReader(gomock.NewController(t))
			loader, err := NewOfficialContextLoader(reader, input.Namespace, bounds)
			require.NoError(t, err)
			ids := []uuid.UUID{input.Entries[0].AccountID}
			if scenario == "invalid entry" {
				input.Entries[1].AccountID = ids[0]
			} else {
				call := reader.EXPECT().Read(gomock.Any(), input.OrganizationID, input.LedgerID, ids, gomock.Any())
				switch scenario {
				case "read error":
					call.Return(nil, nil, errors.New("unavailable"))
				case "extra account":
					input.Accounts = append(input.Accounts, input.Accounts[0])
					call.Return(input.Accounts, input.Assets, nil)
				default:
					call.Return(input.Accounts, input.Assets, nil)
				}
			}
			if scenario == "binding" {
				facts, err := loader.AccountAssets(t.Context(), input.OrganizationID, input.LedgerID, ids)
				require.NoError(t, err)
				require.Len(t, facts, 1)
				require.Equal(t, input.Assets[0].ID, facts[0].Asset.ID)
				return
			}
			result, err := loader.EvaluationContext(t.Context(), input.OrganizationID, input.LedgerID, input.Entries)
			if scenario == "context" {
				require.NoError(t, err)
				require.Len(t, result.Entries, 2)
				require.Equal(t, input.Assets[0].ID, result.Entries[0].Asset.ID)
			} else {
				require.Error(t, err)
				require.Empty(t, result.Entries)
			}
		})
	}
}

func TestOfficialLoaderBatchesRepeatedAccountsAndExternalAssets(t *testing.T) {
	input, bounds := projectionFixture()
	reader := mocks.NewMockOfficialRecordsReader(gomock.NewController(t))
	loader, err := NewOfficialContextLoader(reader, input.Namespace, bounds)
	require.NoError(t, err)
	// Repeated postings (including fees) stay separate; only the database keys deduplicate.
	input.Entries = append(input.Entries, input.Entries[0])
	reader.EXPECT().Read(gomock.Any(), input.OrganizationID, input.LedgerID, []uuid.UUID{input.Entries[0].AccountID}, []string{"BTC"}).Return(input.Accounts, input.Assets, nil).Times(1)
	result, err := loader.EvaluationContext(t.Context(), input.OrganizationID, input.LedgerID, input.Entries)
	require.NoError(t, err)
	require.Len(t, result.Entries, 3)
	require.Len(t, result.Accounts, 1)
	// All-external entries still need official asset identity, but no fake account.
	reader.EXPECT().Read(gomock.Any(), input.OrganizationID, input.LedgerID, []uuid.UUID{}, []string{"BTC"}).Return(nil, input.Assets, nil).Times(1)
	result, err = loader.EvaluationContext(t.Context(), input.OrganizationID, input.LedgerID, []PreparedEntry{input.Entries[1]})
	require.NoError(t, err)
	require.Empty(t, result.Accounts)
	require.Len(t, result.Entries, 1)
}

func TestOfficialLoaderGuardsBeforeIO(t *testing.T) {
	for _, scenario := range []string{"cancelled", "zero scope", "duplicate IDs", "too many entries", "too many accounts", "bad amount", "missing code"} {
		t.Run(scenario, func(t *testing.T) {
			input, bounds := projectionFixture()
			reader := mocks.NewMockOfficialRecordsReader(gomock.NewController(t))
			bounds.MaxAccounts = 1
			bounds.MaxEntries = 2
			loader, err := NewOfficialContextLoader(reader, input.Namespace, bounds)
			require.NoError(t, err)
			ctx := t.Context()
			switch scenario {
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "zero scope":
				input.OrganizationID = uuid.Nil
			case "duplicate IDs":
				_, err := loader.AccountAssets(ctx, input.OrganizationID, input.LedgerID, []uuid.UUID{input.Entries[0].AccountID, input.Entries[0].AccountID})
				require.Error(t, err)
				return
			case "too many entries":
				input.Entries = append(input.Entries, input.Entries[0])
			case "too many accounts":
				input.Entries[1].External = false
				input.Entries[1].AccountID = input.OrganizationID
			case "bad amount":
				input.Entries[0].Amount = decimal.New(1, 1000000000)
			case "missing code":
				input.Entries[0].AssetCode = ""
			}
			_, err = loader.EvaluationContext(ctx, input.OrganizationID, input.LedgerID, input.Entries)
			require.Error(t, err)
		})
	}
}
