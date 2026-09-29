// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestGetFeeDebtSeedsReadsEveryListInOneMGet(t *testing.T) {
	t.Parallel()

	organizationID, ledgerID := uuid.New(), uuid.New()
	payer, payee, fees := "@payer#default", "@payee#default", "@fees#default"
	payerKey, payeeKey := utils.FeeDebtInternalKey(organizationID, ledgerID, payer), utils.FeeDebtInternalKey(organizationID, ledgerID, payee)
	feesKey := utils.FeeDebtInternalKey(organizationID, ledgerID, fees)

	for _, tc := range []struct {
		name   string
		values map[string]string
		err    error
		seeds  map[string][]accounting.FeeDebtItem
		fails  bool
	}{
		{
			name: "open lists come back oldest first and empty ones are omitted",
			values: map[string]string{
				payerKey: `{"v":1,"items":[{"id":"o:from:1:debit","creditRef":"@fees#default"},{"id":"x:from:1:debit","creditRef":"@other#default"}]}`,
				feesKey:  `{"v":1,"items":[]}`,
			},
			seeds: map[string][]accounting.FeeDebtItem{payer: {
				{ID: "o:from:1:debit", CreditRef: "@fees#default"}, {ID: "x:from:1:debit", CreditRef: "@other#default"},
			}},
		},
		{name: "no list is no debt", values: map[string]string{}, seeds: map[string][]accounting.FeeDebtItem{}},
		{name: "an unknown version refuses", values: map[string]string{payeeKey: `{"v":2,"items":[]}`}, fails: true},
		{name: "a malformed list refuses", values: map[string]string{payeeKey: `{"v":1,"items":{}}`}, fails: true},
		{name: "a failed read refuses", err: errors.New("valkey unavailable"), fails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisRepo := redis.NewMockRedisRepository(gomock.NewController(t))
			redisRepo.EXPECT().MGet(gomock.Any(), []string{payerKey, payeeKey, feesKey}).Return(tc.values, tc.err).Times(1)

			seeds, err := (&UseCase{TransactionRedisRepo: redisRepo}).GetFeeDebtSeeds(context.Background(), organizationID, ledgerID, []string{payer, payee, fees})
			if tc.fails {
				require.Error(t, err)
				assert.Nil(t, seeds)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.seeds, seeds)
		})
	}
}
