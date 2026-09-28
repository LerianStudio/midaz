// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

func pendingProjectionTransaction() *transaction.Transaction {
	amount := decimal.NewFromInt(100)
	return &transaction.Transaction{
		ID:             uuid.NewString(),
		OrganizationID: uuid.NewString(),
		LedgerID:       uuid.NewString(),
		AssetCode:      "BRL",
		Amount:         &amount,
		Status:         transaction.Status{Code: constant.PENDING},
		Body: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "BRL",
			Value: amount,
			Source: mtransaction.Source{From: []mtransaction.FromTo{{
				AccountAlias: "@payer", IsFrom: true,
				Amount: &mtransaction.Amount{Asset: "BRL", Value: amount},
			}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
				AccountAlias: "@payee",
				Amount:       &mtransaction.Amount{Asset: "BRL", Value: amount},
			}}},
		}},
	}
}

func TestCreateOrUpdateTransaction_TransitionCASOutcomes(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       string
		transitioned bool
		wantPhase    string
	}{
		{name: "already applied is noop", status: constant.APPROVED, transitioned: false, wantPhase: TransactionLifecyclePhaseNoop},
		{name: "pending row lands", status: constant.CANCELED, transitioned: true, wantPhase: TransactionLifecyclePhaseUpdated},
	} {
		t.Run(test.name, func(t *testing.T) {
			tran := pendingProjectionTransaction()
			tran.Status = transaction.Status{Code: test.status}
			ctrl := gomock.NewController(t)
			repo := transaction.NewMockRepository(ctrl)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).
				Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode})
			repo.EXPECT().UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tran, test.transitioned, nil)

			uc := &UseCase{TransactionRepo: repo}
			logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
			got, phase, err := uc.CreateOrUpdateTransaction(context.Background(), logger, tracer,
				transaction.TransactionProcessingPayload{Transaction: tran, Input: &mtransaction.Transaction{}, Validate: &mtransaction.Responses{Pending: true}})

			require.NoError(t, err)
			assert.Equal(t, test.wantPhase, phase)
			assert.NotNil(t, got)
		})
	}
}

func TestCreateOrUpdateTransaction_LostCASEmitsNothing(t *testing.T) {
	tran := pendingProjectionTransaction()
	tran.Status = transaction.Status{Code: constant.APPROVED}
	ctrl := gomock.NewController(t)
	repo := transaction.NewMockRepository(ctrl)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode})
	repo.EXPECT().UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, false, nil)

	emitter := pkgStreaming.NewMockEmitter()
	uc := &UseCase{TransactionRepo: repo, Streaming: emitter}
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
	got, phase, err := uc.CreateOrUpdateTransaction(context.Background(), logger, tracer,
		transaction.TransactionProcessingPayload{Transaction: tran, Input: &mtransaction.Transaction{}, Validate: &mtransaction.Responses{Pending: true}})

	require.NoError(t, err)
	uc.SendTransactionEvents(context.Background(), got, phase)
	assert.Empty(t, emitter.Events())
}

func TestCreateOrUpdateTransaction_StatusCASErrorPropagates(t *testing.T) {
	tran := pendingProjectionTransaction()
	tran.Status = transaction.Status{Code: constant.CANCELED}
	dbErr := errors.New("status update unavailable")
	ctrl := gomock.NewController(t)
	repo := transaction.NewMockRepository(ctrl)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode})
	repo.EXPECT().UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, false, dbErr)

	uc := &UseCase{TransactionRepo: repo}
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
	got, phase, err := uc.CreateOrUpdateTransaction(context.Background(), logger, tracer,
		transaction.TransactionProcessingPayload{Transaction: tran, Input: &mtransaction.Transaction{}, Validate: &mtransaction.Responses{Pending: true}})

	require.ErrorIs(t, err, dbErr)
	assert.Equal(t, TransactionLifecyclePhaseNoop, phase)
	assert.Nil(t, got)
}
