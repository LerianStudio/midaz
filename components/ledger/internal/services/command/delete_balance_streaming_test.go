// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newDeleteBalanceStreamingTestUseCase wires a happy-path UseCase for
// exercising the balance.deleted emission. The Find returns a
// zero-funds non-internal balance (the only deletable shape); Delete
// returns nil.
func newDeleteBalanceStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
			return &mmodel.Balance{
				ID:             balanceID.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      uuid.New().String(),
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
				Available:      decimal.Zero,
				OnHold:         decimal.Zero,
			}, nil
		}).
		AnyTimes()
	mockBalanceRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	return &UseCase{
		BalanceRepo: mockBalanceRepo,
		Streaming:   emitter,
	}
}

// TestDeleteBalance_EmitsBalanceDeletedEvent verifies that a successful
// DeleteBalance call publishes exactly one balance.deleted event with
// the expected resource/event types and a wall-clock bracketed
// deletedAt.
func TestDeleteBalance_EmitsBalanceDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeleteBalanceStreamingTestUseCase(t, ctrl, mockEmitter)

	orgID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New()

	before := time.Now()
	err := uc.DeleteBalance(context.Background(), orgID, ledgerID, balanceID)
	after := time.Now()
	require.NoError(t, err)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call")
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "balance", "deleted")

	evt := emitted[0]
	assert.Equal(t, "balance.deleted", evt.DefinitionKey)
	assert.Equal(t, balanceID.String(), evt.Subject)

	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)))
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))
	assert.Equal(t, balanceID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.NotEmpty(t, payload["accountId"])
	assert.NotEmpty(t, payload["deletedAt"])
}

// TestDeleteBalance_NoopEmitterDoesNotPanic exercises the
// disabled-flag path.
func TestDeleteBalance_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteBalanceStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())
	err := uc.DeleteBalance(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}

// TestDeleteBalance_EmitFailureDoesNotFailRequest verifies IMPORTANT posture.
func TestDeleteBalance_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteBalanceStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})
	err := uc.DeleteBalance(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err, "Emit failure must NOT fail the request")
}

// TestDeleteBalance_NilStreamingDoesNotPanic exercises the partial-wiring path.
func TestDeleteBalance_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteBalanceStreamingTestUseCase(t, ctrl, nil)
	err := uc.DeleteBalance(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}
