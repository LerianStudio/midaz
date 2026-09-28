// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer/mocks"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestOfficialContextLoader(t *testing.T) {
	for _, scenario := range []string{"context", "read error", "invalid entry", "extra account"} {
		t.Run(scenario, func(t *testing.T) {
			input, bounds := projectionFixture()
			reader := mocks.NewMockOfficialRecordsReader(gomock.NewController(t))
			loader, err := NewOfficialContextLoader(reader, bounds)
			require.NoError(t, err)
			ids := []uuid.UUID{input.Entries[0].AccountID}
			if scenario == "invalid entry" {
				input.Entries[1].AccountID = ids[0]
			} else {
				call := reader.EXPECT().Read(gomock.Any(), input.OrganizationID, input.LedgerID, ids)
				switch scenario {
				case "read error":
					call.Return(nil, errors.New("unavailable"))
				case "extra account":
					input.Accounts = append(input.Accounts, input.Accounts[0])
					call.Return(input.Accounts, nil)
				default:
					call.Return(input.Accounts, nil)
				}
			}
			result, err := loader.EvaluationContext(t.Context(), input.OrganizationID, input.LedgerID, input.Entries)
			if scenario == "context" {
				require.NoError(t, err)
				require.Len(t, result.Entries, 2)
				require.Equal(t, "BTC", result.Entries[0].Asset)
				require.Equal(t, "BTC", result.Accounts[0].Asset)
			} else {
				require.Error(t, err)
				require.Empty(t, result.Entries)
			}
		})
	}
}

func TestOfficialLoaderBatchesRepeatedAccountsAndExternalEntries(t *testing.T) {
	input, bounds := projectionFixture()
	reader := mocks.NewMockOfficialRecordsReader(gomock.NewController(t))
	loader, err := NewOfficialContextLoader(reader, bounds)
	require.NoError(t, err)
	// Repeated postings (including fees) stay separate; only the database keys deduplicate.
	input.Entries = append(input.Entries, input.Entries[0])
	reader.EXPECT().Read(gomock.Any(), input.OrganizationID, input.LedgerID, []uuid.UUID{input.Entries[0].AccountID}).Return(input.Accounts, nil).Times(1)
	result, err := loader.EvaluationContext(t.Context(), input.OrganizationID, input.LedgerID, input.Entries)
	require.NoError(t, err)
	require.Len(t, result.Entries, 3)
	require.Len(t, result.Accounts, 1)
	// All-external entries carry their asset code and need no account read.
	result, err = loader.EvaluationContext(t.Context(), input.OrganizationID, input.LedgerID, []PreparedEntry{input.Entries[1]})
	require.NoError(t, err)
	require.Empty(t, result.Accounts)
	require.Len(t, result.Entries, 1)
	require.Equal(t, "BTC", result.Entries[0].Asset)
}

func TestOfficialLoaderGuardsBeforeIO(t *testing.T) {
	for _, scenario := range []string{"cancelled", "zero scope", "duplicate IDs", "too many entries", "too many accounts", "bad amount", "missing code"} {
		t.Run(scenario, func(t *testing.T) {
			input, bounds := projectionFixture()
			reader := mocks.NewMockOfficialRecordsReader(gomock.NewController(t))
			bounds.MaxAccounts = 1
			bounds.MaxEntries = 2
			loader, err := NewOfficialContextLoader(reader, bounds)
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
				_, err := loader.read(ctx, input.OrganizationID, input.LedgerID, []uuid.UUID{input.Entries[0].AccountID, input.Entries[0].AccountID})
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

func TestOfficialLoaderBoundsEntryAssetCodeByCharacters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		code  string
		valid bool
	}{
		{"legacy lowercase stored code", "usdt", true},
		{"hundred multi-byte characters", strings.Repeat("É", utils.MaxAssetCodeLength), true},
		{"over hundred characters", strings.Repeat("A", utils.MaxAssetCodeLength+1), false},
		{"nul", "US\x00D", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input, bounds := projectionFixture()
			// The generic text bound is smaller than a 100-character multi-byte code.
			bounds.MaxTextBytes = 64
			loader, err := NewOfficialContextLoader(mocks.NewMockOfficialRecordsReader(gomock.NewController(t)), bounds)
			require.NoError(t, err)
			entry := input.Entries[1]
			entry.AssetCode = tc.code

			result, err := loader.EvaluationContext(t.Context(), input.OrganizationID, input.LedgerID, []PreparedEntry{entry})
			if !tc.valid {
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.code, result.Entries[0].Asset)
		})
	}
}
