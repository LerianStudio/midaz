// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/v2/metrics"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func collectBlockedRejectionCounter(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var total int64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "blocked_account_rejections_total" {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}

	return total
}

// TestProcessBalanceOperations_BlockedAccountRejectionCounter proves a 0502
// validation failure (blocked source, no exception grant) increments
// blocked_account_rejections_total, and that a NON-0502 validation failure
// (asset mismatch) does NOT.
func TestProcessBalanceOperations_BlockedAccountRejectionCounter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	newUC := func() (*sdkmetric.ManualReader, UseCase) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

		factory, err := metrics.NewMetricsFactory(mp.Meter("blocked-rejection-test-"+uuid.NewString()), nil)
		require.NoError(t, err)

		return reader, UseCase{
			TransactionRedisRepo: redis.NewMockRedisRepository(ctrl),
			MetricsFactory:       factory,
		}
	}

	buildInput := func(blocked bool, balanceAsset string) ProcessBalanceOperationsInput {
		fromAmount := mtransaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100), Operation: constant.DEBIT}

		validate := &mtransaction.Responses{
			Asset:   "BRL",
			Aliases: []string{"@acc#default"},
			From:    map[string]mtransaction.Amount{"0#@acc#default": fromAmount},
		}

		bal := &mmodel.Balance{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "@acc",
			Key:            "default",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AccountBlocked: blocked,
			AssetCode:      balanceAsset,
		}

		return ProcessBalanceOperationsInput{
			OrganizationID:    organizationID,
			LedgerID:          ledgerID,
			TransactionID:     uuid.New(),
			TransactionInput:  &mtransaction.Transaction{Send: mtransaction.Send{Asset: "BRL"}},
			Validate:          validate,
			BalanceOperations: []mmodel.BalanceOperation{{Balance: bal, Alias: "0#@acc#default", Amount: fromAmount, InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "@acc#default")}},
			TransactionStatus: constant.CREATED,
		}
	}

	t.Run("0502 blocked account increments counter", func(t *testing.T) {
		reader, uc := newUC()

		_, err := uc.ProcessBalanceOperations(ctx, buildInput(true, "BRL"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), constant.ErrAccountBlockedTransactionRestriction.Error())
		assert.Equal(t, int64(1), collectBlockedRejectionCounter(t, reader), "0502 must count a blocked-account rejection")
	})

	t.Run("non-0502 failure does not increment counter", func(t *testing.T) {
		reader, uc := newUC()

		_, err := uc.ProcessBalanceOperations(ctx, buildInput(false, "USD")) // asset mismatch => not 0502
		require.Error(t, err)
		assert.Equal(t, int64(0), collectBlockedRejectionCounter(t, reader), "non-0502 must not count a blocked-account rejection")
	})
}
