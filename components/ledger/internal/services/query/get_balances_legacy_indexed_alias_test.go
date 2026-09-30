// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestGetBalancesFromCache_ServesLegacyIndexedAliasWithoutDatabaseFallback covers
// blobs cached by 4.0.x and 3.8.x, whose Alias carries the transaction entry key
// ("<index>#<alias>#<key>"). They must be served from the cache: the balance
// repository mock has no expectations, so a fallback read fails the test.
func TestGetBalancesFromCache_ServesLegacyIndexedAliasWithoutDatabaseFallback(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("5a1f5c1e-4f6b-4c62-9d0e-0d6f3b7c2a11")
	ledgerID := uuid.MustParse("7b2e6d2f-5a7c-4d73-8e1f-1e7a4c8d3b22")
	balanceID := uuid.MustParse("8c3f7e3a-6b8d-4e84-9f2a-2f8b5d9e4c33")
	accountID := uuid.MustParse("9d4a8f4b-7c9e-4f95-8a3b-3a9c6e1f5d44")

	for _, tc := range []struct {
		name          string
		blob          string
		wantAvailable decimal.Decimal
		wantVersion   int64
	}{
		{
			name: "pure legacy blob",
			blob: fmt.Sprintf(`{"ID":%q,"AccountID":%q,"AccountType":"deposit","AssetCode":"USD",`+
				`"Alias":"0#@source#default","Key":"default","Available":"10","OnHold":"0","Version":7,`+
				`"AllowSending":1,"AllowReceiving":1,"Direction":"credit","OverdraftUsed":"0",`+
				`"AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`,
				balanceID, accountID),
			wantAvailable: decimal.NewFromInt(10),
			wantVersion:   7,
		},
		{
			name: "mixed blob keeps uppercase authority",
			blob: fmt.Sprintf(`{"SchemaVersion":2,`+
				`"ID":%[1]q,"AccountID":%[2]q,"AccountType":"deposit","AssetCode":"USD","Alias":"12#@source#default","Key":"default",`+
				`"Available":"999","OnHold":"0","Version":7,"AllowSending":1,"AllowReceiving":1,`+
				`"id":%[1]q,"accountId":%[2]q,"accountType":"deposit","assetCode":"USD","alias":"@source","key":"default",`+
				`"available":"1000","onHold":"0","version":"1","allowSending":true,"allowReceiving":true}`,
				balanceID, accountID),
			wantAvailable: decimal.NewFromInt(999),
			wantVersion:   7,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			uc := &UseCase{
				TransactionRedisRepo: mockRedisRepo,
				BalanceRepo:          balance.NewMockRepository(ctrl),
			}

			mockRedisRepo.EXPECT().
				Get(gomock.Any(), utils.BalanceInternalKey(organizationID, ledgerID, "@source#default")).
				Return(tc.blob, nil).
				Times(1)

			balances, err := uc.GetBalances(t.Context(), organizationID, ledgerID, []string{"@source#default"})
			require.NoError(t, err)
			require.Len(t, balances, 1)

			got := balances[0]
			require.Equal(t, balanceID.String(), got.ID)
			require.Equal(t, accountID.String(), got.AccountID)
			require.Equal(t, "@source", got.Alias)
			require.Equal(t, "default", got.Key)
			require.True(t, tc.wantAvailable.Equal(got.Available), "available = %s, want %s", got.Available, tc.wantAvailable)
			require.Equal(t, tc.wantVersion, got.Version)
		})
	}
}
