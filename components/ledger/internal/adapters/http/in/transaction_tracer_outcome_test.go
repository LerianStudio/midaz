// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

type financialRedisDurabilityStub struct {
	err   error
	calls int
}

func (s *financialRedisDurabilityStub) FinancialDurability(context.Context) error {
	s.calls++
	return s.err
}

func TestPrepareTracerOutcomeDelegatesAtomicTenantPrepare(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)
	handler := &TransactionHandler{
		Command:            &command.UseCase{TransactionRedisRepo: repo},
		MultiTenantEnabled: true,
	}
	const tenantID = "tenant-restart-safe"
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.New()
	attempt := &mmodel.BalanceExecutionAttempt{Owner: "owner", TracerOutcomeID: uuid.New()}
	plan := &mmodel.ExpectedEconomicPlan{Version: 1, Digest: "digest"}

	repo.EXPECT().PrepareTracerOutcome(gomock.Any(), organizationID, ledgerID, transactionID,
		attempt.Owner, attempt.TracerOutcomeID, plan, gomock.Any(), gomock.Any()).
		DoAndReturn(func(repoCtx context.Context, _, _, _ uuid.UUID, _ string, _ uuid.UUID,
			_ *mmodel.ExpectedEconomicPlan, preparedAt, _ time.Time) (*mmodel.TracerOutcomeRecord, error) {
			require.Equal(t, tenantID, tmcore.GetTenantIDContext(repoCtx))
			return &mmodel.TracerOutcomeRecord{State: mmodel.TracerOutcomePrepared, PreparedAtUnixMS: preparedAt.UnixMilli()}, nil
		})

	require.NoError(t, handler.prepareTracerOutcome(ctx, organizationID, ledgerID, transactionID, attempt, plan))
}

func TestPrepareTracerOutcomeRequiresTenantContextBeforeRedis(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	handler := &TransactionHandler{
		Command:            &command.UseCase{TransactionRedisRepo: redisTransaction.NewMockRedisRepository(ctrl)},
		MultiTenantEnabled: true,
	}

	err := handler.prepareTracerOutcome(context.Background(), uuid.New(), uuid.New(), uuid.New(),
		&mmodel.BalanceExecutionAttempt{Owner: "owner", TracerOutcomeID: uuid.New()},
		&mmodel.ExpectedEconomicPlan{Version: 1, Digest: "digest"})
	require.ErrorContains(t, err, "tenant context is required")
}

func TestDurableTracerOutcomeAdmissionFailsClosedOnUnsafeRedis(t *testing.T) {
	t.Parallel()

	unsafe := &financialRedisDurabilityStub{err: errors.New("appendonly must be enabled")}
	handler := &TransactionHandler{FinancialRedisDurability: unsafe}
	require.ErrorContains(t, handler.admitDurableTracerOutcome(context.Background()), "appendonly must be enabled")
	require.Equal(t, 1, unsafe.calls)

	require.Error(t, (&TransactionHandler{}).admitDurableTracerOutcome(context.Background()),
		"missing admission guard must fail closed")
}
