// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// errBalancesUnavailable stands in for any failure past the version-specific gates.
// A pipeline that reaches it proves the earlier gates did NOT reject the request.
var errBalancesUnavailable = errors.New("balances unavailable")

// versionReader is a TransactionReader whose settings carry no skip opt-in and whose
// balance read always fails, so a create that gets past the skip gate stops at a
// recognisable error instead of needing the whole persistence stack.
type versionReader struct {
	settings         mmodel.LedgerSettings
	getBalancesCalls int
}

func (r *versionReader) GetParsedLedgerSettings(context.Context, uuid.UUID, uuid.UUID) (mmodel.LedgerSettings, error) {
	return r.settings, nil
}

func (r *versionReader) GetBalances(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
	r.getBalancesCalls++

	return nil, errBalancesUnavailable
}

func (r *versionReader) ValidateAccountingRules(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
	return nil, nil
}

// skippingTransaction is a minimal well-formed transaction that asks for both per-call
// control skips.
func skippingTransaction() mtransaction.Transaction {
	return mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "BRL",
			Value: decimal.NewFromInt(100),
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{
					AccountAlias: "@payer",
					Amount:       &mtransaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
				}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{
					AccountAlias: "@payee",
					Amount:       &mtransaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(100)},
				}},
			},
		},
		Skip: &mtransaction.TransactionSkip{Fees: true, Tracer: true},
	}
}

// newVersionUseCase wires a UseCase whose Redis slot is free (SetNX succeeds) and whose
// queue accepts the seed, so the pipeline runs from the idempotency claim to the balance
// read. It returns the reader so a test can assert whether the read was reached.
func newVersionUseCase(t *testing.T, settings mmodel.LedgerSettings) (*UseCase, *versionReader, *txRedis.MockRedisRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	reader := &versionReader{settings: settings}

	return &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}, reader, redisRepo
}

// TestCreateTransactionV1_SkipIsInert proves the /v1 contract carries no per-call skip
// control: a body asking for both skips on a ledger that opts into neither is NOT
// rejected, and the request continues to the balance read.
func TestCreateTransactionV1_SkipIsInert(t *testing.T) {
	uc, reader, redisRepo := newVersionUseCase(t, mmodel.LedgerSettings{})

	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	redisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	_, replayed, err := uc.CreateTransactionV1(context.Background(), CreateTransactionV1Input{
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Transaction:       skippingTransaction(),
		TransactionStatus: constant.CREATED,
		IdempotencyTTL:    time.Minute,
	})

	require.ErrorIs(t, err, errBalancesUnavailable, "a /v1 create must not be rejected for asking to skip a control it does not have")
	assert.False(t, replayed)
	assert.Equal(t, 1, reader.getBalancesCalls, "the /v1 pipeline must reach the balance read")
}

// TestCreateTransactionV2_SkipWithoutOptInRejects is the counterpart: the /v2 contract
// does carry the controls, so a skip the ledger never opted into is a 422 that releases
// the idempotency claim and never reaches the balance read.
func TestCreateTransactionV2_SkipWithoutOptInRejects(t *testing.T) {
	uc, reader, redisRepo := newVersionUseCase(t, mmodel.LedgerSettings{})

	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	_, _, err := uc.CreateTransactionV2(context.Background(), CreateTransactionV2Input{
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Transaction:       skippingTransaction(),
		TransactionStatus: constant.CREATED,
		IdempotencyTTL:    time.Minute,
	})

	require.Error(t, err)

	var business pkg.UnprocessableOperationError

	require.ErrorAs(t, err, &business)
	assert.Equal(t, constant.ErrSkipNotPermitted.Error(), business.Code)
	assert.Zero(t, reader.getBalancesCalls, "the 422 must precede the balance read")
}

// TestCreateTransactionV2_SkipWithOptInProceeds proves the /v2 rejection is the two-key
// gate biting, not the control being unreachable: with the opt-in the same body runs on.
func TestCreateTransactionV2_SkipWithOptInProceeds(t *testing.T) {
	settings := mmodel.LedgerSettings{}
	settings.Overrides.AllowFeeSkip = true
	settings.Overrides.AllowTracerSkip = true

	uc, reader, redisRepo := newVersionUseCase(t, settings)

	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	redisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	_, _, err := uc.CreateTransactionV2(context.Background(), CreateTransactionV2Input{
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Transaction:       skippingTransaction(),
		TransactionStatus: constant.CREATED,
		IdempotencyTTL:    time.Minute,
	})

	require.ErrorIs(t, err, errBalancesUnavailable)
	assert.Equal(t, 1, reader.getBalancesCalls)
}

// TestCreateTransaction_ReplayShortCircuits proves both pipelines answer a claimed
// idempotency slot with the cached transaction and never touch the read path.
func TestCreateTransaction_ReplayShortCircuits(t *testing.T) {
	cached := &transaction.Transaction{ID: uuid.New().String()}

	raw, err := json.Marshal(cached)
	require.NoError(t, err)

	newReplayUseCase := func(t *testing.T) (*UseCase, *versionReader) {
		t.Helper()

		ctrl := gomock.NewController(t)
		redisRepo := txRedis.NewMockRedisRepository(ctrl)

		redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
		redisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return(string(raw), nil).AnyTimes()

		reader := &versionReader{}

		return &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}, reader
	}

	t.Run("v1", func(t *testing.T) {
		uc, reader := newReplayUseCase(t)

		tran, replayed, err := uc.CreateTransactionV1(context.Background(), CreateTransactionV1Input{
			OrganizationID:    uuid.New(),
			LedgerID:          uuid.New(),
			Transaction:       skippingTransaction(),
			TransactionStatus: constant.CREATED,
			IdempotencyTTL:    time.Minute,
		})

		require.NoError(t, err)
		assert.True(t, replayed)
		assert.Equal(t, cached.ID, tran.ID)
		assert.Zero(t, reader.getBalancesCalls)
	})

	t.Run("v2", func(t *testing.T) {
		uc, reader := newReplayUseCase(t)

		tran, replayed, err := uc.CreateTransactionV2(context.Background(), CreateTransactionV2Input{
			OrganizationID:    uuid.New(),
			LedgerID:          uuid.New(),
			Transaction:       skippingTransaction(),
			TransactionStatus: constant.CREATED,
			IdempotencyTTL:    time.Minute,
		})

		require.NoError(t, err)
		assert.True(t, replayed)
		assert.Equal(t, cached.ID, tran.ID)
		assert.Zero(t, reader.getBalancesCalls)
	})
}
