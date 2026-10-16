// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	idempotencySpanName       = "command.create_idempotency_key"
	additionalBalanceSpanName = "command.create_additional_balance"
)

// conflictKeyContext wires a span recorder and a capturing logger into the tracking context.
func conflictKeyContext() (context.Context, *tracetest.SpanRecorder, *capturingLogger) {
	ctx, recorder := spantest.RecordingContext()
	logger := &capturingLogger{}

	return libObservability.ContextWithLogger(ctx, logger), recorder, logger
}

func TestCreateOrCheckTransactionIdempotency_KeyIsASpanAttribute(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	ttl := 24 * time.Hour

	tests := []struct {
		name         string
		key          string
		hash         string
		wantKey      string
		wantExplicit bool
	}{
		{name: "explicit key", key: "client-key", hash: "body-hash", wantKey: "client-key", wantExplicit: true},
		{name: "key derived from the body hash", key: "", hash: "body-hash", wantKey: "body-hash", wantExplicit: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			mockRedisRepo.EXPECT().
				SetNX(gomock.Any(), utils.IdempotencyInternalKey(organizationID, ledgerID, tt.wantKey), "", ttl).
				Return(true, nil).
				Times(1)

			uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

			ctx, recorder, _ := conflictKeyContext()

			_, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, tt.key, tt.hash, "fingerprint", ttl)
			require.NoError(t, err)

			span := spantest.FindSpan(t, recorder, idempotencySpanName)

			key, ok := spanAttribute(span, "app.request.idempotency_key")
			require.True(t, ok, "span must carry the effective idempotency key")
			assert.Equal(t, tt.wantKey, key.AsString())

			explicit, ok := spanAttribute(span, "app.request.idempotency_key_explicit")
			require.True(t, ok, "span must carry whether the key was explicit")
			assert.Equal(t, tt.wantExplicit, explicit.AsBool())
		})
	}
}

func TestCreateOrCheckTransactionIdempotency_ConflictIsASpanEventWithoutALog(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	ttl := 24 * time.Hour
	key := "client-key"
	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

	tests := []struct {
		name        string
		storedValue func(t *testing.T) string
		wantEvent   string
	}{
		{
			name:        "key already in use while the first request is in flight",
			storedValue: func(_ *testing.T) string { return "" },
			wantEvent:   "Idempotency key already in use",
		},
		{
			name: "key reused with a different request",
			storedValue: func(t *testing.T) string {
				return slotValueWithFingerprint(t, transaction.Transaction{ID: uuid.New().String()}, "first-request")
			},
			wantEvent: "Idempotency key reused with a different request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			mockRedisRepo.EXPECT().SetNX(gomock.Any(), internalKey, "", ttl).Return(false, nil).Times(1)
			mockRedisRepo.EXPECT().Get(gomock.Any(), internalKey).Return(tt.storedValue(t), nil).Times(1)

			uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

			ctx, recorder, logger := conflictKeyContext()

			_, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, "body-hash", "second-request", ttl)

			var conflict pkg.EntityConflictError
			require.ErrorAs(t, err, &conflict)
			assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)

			span := spantest.FindSpan(t, recorder, idempotencySpanName)
			assert.Equal(t, codes.Unset, span.Status().Code)

			event, ok := spantest.FindEvent(span, tt.wantEvent)
			require.True(t, ok, "conflict must be recorded as a business event")
			assert.Contains(t, spantest.EventText(event), err.Error())

			keyAttr, ok := spanAttribute(span, "app.request.idempotency_key")
			require.True(t, ok)
			assert.Equal(t, key, keyAttr.AsString())

			assert.Empty(t, logger.atLevelOrMoreSevere(libLog.LevelWarn), "a conflict must not be logged at Warn or above")
		})
	}
}

func TestCreateAdditionalBalance_KeyIsASpanAttribute(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	defaultBalance := &mmodel.Balance{
		ID:             uuid.New().String(),
		Alias:          "test-alias",
		Key:            constant.DefaultBalanceKey,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		AssetCode:      "USD",
		AccountType:    "deposit",
	}

	t.Run("success carries the lower-cased key", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "asset-freeze").
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
			Times(1)
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(defaultBalance, nil).
			Times(1)
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) { return b, nil }).
			Times(1)

		uc := &UseCase{BalanceRepo: mockBalanceRepo}

		ctx, recorder, _ := conflictKeyContext()

		_, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, &mmodel.CreateAdditionalBalance{Key: "Asset-Freeze"})
		require.NoError(t, err)

		span := spantest.FindSpan(t, recorder, additionalBalanceSpanName)

		key, ok := spanAttribute(span, "app.request.balance_key")
		require.True(t, ok, "span must carry the balance key")
		assert.Equal(t, "asset-freeze", key.AsString())
	})

	t.Run("conflict carries the key and a business event with the error", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "duplicate-key").
			Return(&mmodel.Balance{Key: "duplicate-key"}, nil).
			Times(1)

		uc := &UseCase{BalanceRepo: mockBalanceRepo}

		ctx, recorder, logger := conflictKeyContext()

		result, err := uc.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, &mmodel.CreateAdditionalBalance{Key: "duplicate-key"})
		assert.Nil(t, result)

		var conflict pkg.EntityConflictError
		require.ErrorAs(t, err, &conflict)
		assert.Equal(t, constant.ErrDuplicatedAliasKeyValue.Error(), conflict.Code)

		span := spantest.FindSpan(t, recorder, additionalBalanceSpanName)
		assert.Equal(t, codes.Unset, span.Status().Code)

		key, ok := spanAttribute(span, "app.request.balance_key")
		require.True(t, ok, "span must carry the balance key on conflict")
		assert.Equal(t, "duplicate-key", key.AsString())

		event, ok := spantest.FindEvent(span, "Additional balance already exists")
		require.True(t, ok, "conflict must be recorded as a business event")
		assert.Contains(t, spantest.EventText(event), err.Error(), "the event must carry the business error")

		assert.Empty(t, logger.atLevelOrMoreSevere(libLog.LevelWarn), "a conflict must not be logged at Warn or above")
	})
}
