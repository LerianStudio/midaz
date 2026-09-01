// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestGetBalances_AccountBlockedMapping locks the cache->domain projection of
// the account block flag. The cache carries it as the same 1/0 integer the Lua
// script uses for the sibling allow flags, so the mapping must be the identical
// `== 1` comparison — and a legacy cache entry written before the field existed
// must decode to false rather than fail the read.
func TestGetBalances_AccountBlockedMapping(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name     string
		cacheRaw func(t *testing.T) string
		want     bool
	}{
		{
			name: "blocked account maps to true",
			cacheRaw: func(t *testing.T) string {
				t.Helper()

				raw, err := json.Marshal(mmodel.BalanceRedis{
					ID:             uuid.New().String(),
					AccountID:      uuid.New().String(),
					Available:      decimal.NewFromInt(100),
					OnHold:         decimal.Zero,
					Version:        1,
					AccountType:    "deposit",
					AllowSending:   1,
					AllowReceiving: 1,
					AccountBlocked: 1,
					AssetCode:      "USD",
				})
				require.NoError(t, err)

				return string(raw)
			},
			want: true,
		},
		{
			name: "unblocked account maps to false",
			cacheRaw: func(t *testing.T) string {
				t.Helper()

				raw, err := json.Marshal(mmodel.BalanceRedis{
					ID:             uuid.New().String(),
					AccountID:      uuid.New().String(),
					Available:      decimal.NewFromInt(100),
					OnHold:         decimal.Zero,
					Version:        1,
					AccountType:    "deposit",
					AllowSending:   1,
					AllowReceiving: 1,
					AccountBlocked: 0,
					AssetCode:      "USD",
				})
				require.NoError(t, err)

				return string(raw)
			},
			want: false,
		},
		{
			name: "legacy cache entry without the field maps to false",
			cacheRaw: func(t *testing.T) string {
				t.Helper()

				// Written by a build that predates the field entirely.
				return `{"id":"` + uuid.New().String() + `","accountId":"` + uuid.New().String() +
					`","available":"100","onHold":"0","version":1,"accountType":"deposit",` +
					`"allowSending":1,"allowReceiving":1,"assetCode":"USD"}`
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			uc := &UseCase{
				BalanceRepo:          mockBalanceRepo,
				TransactionRedisRepo: mockRedisRepo,
			}

			alias := "@blocked#default"
			internalKey := utils.BalanceInternalKey(organizationID, ledgerID, alias)

			mockRedisRepo.EXPECT().
				Get(gomock.Any(), internalKey).
				Return(tc.cacheRaw(t), nil).
				Times(1)

			balances, err := uc.GetBalances(context.Background(), organizationID, ledgerID, []string{alias})

			require.NoError(t, err)
			require.Len(t, balances, 1)
			assert.Equal(t, tc.want, balances[0].AccountBlocked,
				"AccountBlocked must be projected from the cache 1/0 mirror")
		})
	}
}
