// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// revertReader answers the three reads the revert eligibility gate performs and records
// whether the pipeline behind the gate was reached (GetParsedLedgerSettings is the first
// read the create pipeline makes that the gate does not).
type revertReader struct {
	versionReader

	parent        *transaction.Transaction
	parentErr     error
	origin        *transaction.Transaction
	originErr     error
	operationRout *mmodel.OperationRoute
	routeErr      error
}

func (r *revertReader) GetParentByTransactionID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*transaction.Transaction, error) {
	return r.parent, r.parentErr
}

func (r *revertReader) GetTransactionWithOperationsByID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*transaction.Transaction, error) {
	return r.origin, r.originErr
}

func (r *revertReader) GetOperationRouteByID(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, uuid.UUID) (*mmodel.OperationRoute, error) {
	return r.operationRout, r.routeErr
}

// revertibleOrigin builds an APPROVED transaction with one unrouted operation pair, the
// minimum TransactionRevert needs to produce a non-empty reversal.
func revertibleOrigin() *transaction.Transaction {
	amount := decimal.NewFromInt(100)

	return &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		AssetCode:      "BRL",
		Amount:         &amount,
		Status:         transaction.Status{Code: constant.APPROVED},
		Operations: []*operation.Operation{
			{Type: constant.DEBIT, AccountAlias: "@payer", Amount: operation.Amount{Value: &amount}, AssetCode: "BRL", BalanceKey: "default"},
			{Type: constant.CREDIT, AccountAlias: "@payee", Amount: operation.Amount{Value: &amount}, AssetCode: "BRL", BalanceKey: "default"},
		},
	}
}

// newRevertUseCase wires a UseCase whose idempotency slot is free and whose reader
// answers the eligibility gate, so a revert that passes the gate runs on into the create
// pipeline and stops at the balance read.
func newRevertUseCase(t *testing.T, reader *revertReader) *UseCase {
	t.Helper()

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	return &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}
}

func revertInput() RevertTransactionInput {
	return RevertTransactionInput{OrganizationID: uuid.New(), LedgerID: uuid.New(), TransactionID: uuid.New()}
}

// TestRevertTransaction_EligibilityGateIsShared proves both versions run the SAME
// eligibility gate: an origin that already has a parent is rejected with the same
// business error on either contract, before any pipeline step.
func TestRevertTransaction_EligibilityGateIsShared(t *testing.T) {
	for _, tc := range []struct {
		name   string
		invoke func(uc *UseCase, in RevertTransactionInput) error
	}{
		{name: "v1", invoke: func(uc *UseCase, in RevertTransactionInput) error {
			_, _, err := uc.RevertTransactionV1(context.Background(), in)

			return err
		}},
		{name: "v2", invoke: func(uc *UseCase, in RevertTransactionInput) error {
			_, _, err := uc.RevertTransactionV2(context.Background(), in)

			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &revertReader{parent: &transaction.Transaction{ID: uuid.New().String()}}
			uc := newRevertUseCase(t, reader)

			err := tc.invoke(uc, revertInput())

			require.Error(t, err)

			var business pkg.EntityConflictError

			require.ErrorAs(t, err, &business)
			assert.Equal(t, constant.ErrTransactionIDHasAlreadyParentTransaction.Error(), business.Code)
			assert.Zero(t, reader.getBalancesCalls, "the eligibility gate must reject before the pipeline runs")
		})
	}
}

// TestRevertTransactionV1_NeverDialsTheTracer proves the /v1 revert reaches the balance
// read with a reserver that fails the test on any call: the /v1 pipeline never asks the
// tracer anything.
func TestRevertTransactionV1_NeverDialsTheTracer(t *testing.T) {
	reader := &revertReader{origin: revertibleOrigin()}
	uc := newRevertUseCase(t, reader)
	uc.TracerReserver = &forbiddenReserver{t: t}

	_, _, err := uc.RevertTransactionV1(context.Background(), revertInput())

	require.ErrorIs(t, err, errBalancesUnavailable, "the /v1 revert must reach the balance read")
	assert.Equal(t, 1, reader.getBalancesCalls)
}

// TestRevertTransactionV2_NeverTouchesOriginReservation proves a /v2 revert that aborts
// before its own reserve anchor still leaves the ORIGIN's reservation alone: nothing is
// released or confirmed on the way through. That a /v2 revert does reserve for the
// REVERSE transaction is proven by TestRevertNoReservationRefund and by the source gate
// TestRevertV2_NeverAppliesFees.
func TestRevertTransactionV2_NeverTouchesOriginReservation(t *testing.T) {
	reader := &revertReader{origin: revertibleOrigin()}
	reader.settings.Tracer = mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}

	reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{uuid.New()}}}

	uc := newRevertUseCase(t, reader)
	uc.TracerReserver = reserver

	_, _, err := uc.RevertTransactionV2(context.Background(), revertInput())

	require.ErrorIs(t, err, errBalancesUnavailable, "the /v2 revert must reach the balance read")
	assert.Equal(t, 0, reserver.reserveCalls, "the reserve anchor sits after the balance staging, which failed here")

	assert.Empty(t, reserver.releasedIDs, "a revert must never release the origin's reservation")
	assert.Empty(t, reserver.confirmedIDs, "a revert must never confirm the origin's reservation")
}
