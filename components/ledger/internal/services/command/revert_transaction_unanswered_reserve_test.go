// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestRevertTransactionV2_UnansweredFailOpenReserveConfirmsByTransaction sets
// AUDIT_LOG_ENABLED through t.Setenv, so it cannot run in parallel.
func TestRevertTransactionV2_UnansweredFailOpenReserveConfirmsByTransaction(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	idempotencySet := make(chan struct{})
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Duration(300)).Return(true, nil).Times(1)
	redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(300)).DoAndReturn(
		func(context.Context, string, string, time.Duration) error {
			close(idempotencySet)
			return nil
		},
	).Times(1)

	organizationID := uuid.MustParse("91111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("92222222-2222-4222-8222-222222222222")
	originID := uuid.MustParse("93333333-3333-4333-8333-333333333333")

	settings := mmodel.LedgerSettings{}
	settings.Tracer = mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}
	reader := &revertEngineReader{
		revertReader: &revertReader{
			origin:        revertEngineOrigin(organizationID, ledgerID, originID),
			versionReader: versionReader{settings: settings},
		},
		balances: []*mmodel.Balance{
			revertEngineBalance(organizationID, ledgerID, "94444444-4444-4444-8444-444444444444", "@payee", 50, 7),
			revertEngineBalance(organizationID, ledgerID, "95555555-5555-4555-8555-555555555555", "@payer", 20, 3),
		},
	}
	executor := &revertLiteralEngine{t: t}
	reserver := &stubReserver{reserveErr: errReserveDeadline, confirmByTxnOutcome: tracer.ConfirmOutcome{Confirmed: 1}}
	uc := &UseCase{
		TransactionRedisRepo: redisRepo, TransactionReader: reader, Engine: executor,
		AppliedTransactionCompleter: &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}},
		EngineRecoveryAcknowledger:  &recordingEngineRecoveryAcknowledger{},
		TracerReserver:              reserver,
	}
	settles := withImmediateUnansweredSettles(t, uc)

	got, replayed, err := uc.RevertTransactionV2(tmcore.ContextWithTenantID(context.Background(), "tenant-revert-unanswered"),
		RevertTransactionInput{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: originID})
	require.NoError(t, err)
	settles.drain()
	assert.False(t, replayed)
	require.NotNil(t, got)
	require.Len(t, executor.requests, 1)

	requests := reserver.reserveRequests()
	require.Len(t, requests, 1)
	assert.True(t, requests[0].Revert)

	revertID := uuid.MustParse(got.ID)
	assert.Equal(t, revertID, requests[0].TransactionID, "the revert reserves for its own transaction")
	assert.Equal(t, []uuid.UUID{revertID}, reserver.confirmedTransactions(),
		"the applied revert counts its own spend once, by its own transaction id")
	assert.NotContains(t, reserver.confirmedTransactions(), originID, "the origin's reservation is never touched")
	assert.Empty(t, reserver.releasedTransactions())
	assert.Empty(t, reserver.confirmed())
	assert.Empty(t, reserver.released())

	select {
	case <-idempotencySet:
	case <-time.After(time.Second):
		t.Fatal("the revert did not populate the idempotency value")
	}
}
