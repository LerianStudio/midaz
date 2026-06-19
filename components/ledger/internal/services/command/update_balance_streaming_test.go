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
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	txRedis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// updateBalanceStreamingFixture wraps the inputs the test scenarios
// need to share: the in-memory parent balance returned by Find, and
// the post-update balance returned by Update. Keeping them as fields
// on the fixture avoids parallel mocks drifting out of sync.
type updateBalanceStreamingFixture struct {
	current *mmodel.Balance
	updated *mmodel.Balance
}

func defaultUpdateBalanceStreamingFixture() updateBalanceStreamingFixture {
	id := uuid.New().String()
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	updatedAt := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)

	current := &mmodel.Balance{
		ID:             id,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		Alias:          "@cash",
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "USD",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionCredit,
	}

	updated := *current
	updated.AllowSending = false
	updated.UpdatedAt = updatedAt

	return updateBalanceStreamingFixture{current: current, updated: &updated}
}

func newUpdateBalanceStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixture updateBalanceStreamingFixture) *UseCase {
	t.Helper()

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fixture.current, nil).
		AnyTimes()
	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fixture.updated, nil).
		AnyTimes()

	// The cache overlay path is best-effort and reads/writes regardless
	// of the update content. Empty Get + a swallowed write is enough.
	mockRedis := txRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		AnyTimes()
	mockRedis.EXPECT().
		UpdateBalanceCacheSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	return &UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedis,
		Streaming:            emitter,
	}
}

// TestUpdateBalance_EmitsConfigChangedSettingsUpdated verifies that a
// regular settings PATCH emits a single balance.config_changed event
// with changeType=settings_updated for the parent balance.
func TestUpdateBalance_EmitsConfigChangedSettingsUpdated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	fixture := defaultUpdateBalanceStreamingFixture()
	uc := newUpdateBalanceStreamingTestUseCase(t, ctrl, mockEmitter, fixture)

	allowSending := false
	update := mmodel.UpdateBalance{AllowSending: &allowSending}

	out, err := uc.Update(context.Background(), uuid.New(), uuid.New(), uuid.MustParse(fixture.updated.ID), update)
	require.NoError(t, err)
	require.NotNil(t, out)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call when no overdraft transition occurs")
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "balance", "config-changed")

	evt := emitted[0]
	assert.Equal(t, "balance.config-changed", evt.DefinitionKey)
	assert.Equal(t, fixture.updated.ID, evt.Subject)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))
	assert.Equal(t, "settings_updated", payload["changeType"])
	assert.Equal(t, fixture.updated.ID, payload["id"])
	assert.Equal(t, fixture.updated.AccountID, payload["accountId"])
	assert.Equal(t, false, payload["allowSending"])
}

// TestUpdateBalance_EmitsTwoEventsOnOverdraftTransition verifies that a
// PATCH flipping AllowOverdraft false->true emits TWO config_changed
// events: first for the auto-created companion (overdraft_enabled)
// then for the parent (settings_updated).
func TestUpdateBalance_EmitsTwoEventsOnOverdraftTransition(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()

	// Build a custom fixture: parent currently has AllowOverdraft=false
	// and a valid AccountID UUID so ensureOverdraftBalance can parse it.
	id := uuid.New().String()
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	updatedAt := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)

	current := &mmodel.Balance{
		ID:             id,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		Alias:          "@cash",
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "USD",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionCredit,
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft: false,
		},
	}

	updated := *current
	updated.UpdatedAt = updatedAt
	updated.Settings = &mmodel.BalanceSettings{AllowOverdraft: true}

	companionID := uuid.New().String()
	companion := &mmodel.Balance{
		ID:             companionID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		Alias:          "@cash",
		Key:            constant.OverdraftBalanceKey,
		AssetCode:      "USD",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionDebit,
		Settings:       &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal},
		UpdatedAt:      updatedAt,
		CreatedAt:      updatedAt,
		OverdraftUsed:  decimal.Zero,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(current, nil).AnyTimes()
	mockBalanceRepo.EXPECT().FindByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.OverdraftBalanceKey).Return(nil, pkg.EntityNotFoundError{EntityType: constant.EntityBalance}).Times(1)
	mockBalanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(companion, nil).Times(1)
	mockBalanceRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&updated, nil).AnyTimes()

	mockRedis := txRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	mockRedis.EXPECT().UpdateBalanceCacheSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	uc := &UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedis,
		Streaming:            mockEmitter,
	}

	allowOverdraft := true
	limit := "1000"
	update := mmodel.UpdateBalance{
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        allowOverdraft,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        &limit,
		},
	}

	out, err := uc.Update(context.Background(), uuid.MustParse(orgID), uuid.MustParse(ledgerID), uuid.MustParse(id), update)
	require.NoError(t, err)
	require.NotNil(t, out)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 2, "overdraft transition must emit companion + parent config_changed events")

	// Event 0: companion's config_changed{overdraft_enabled}
	var payload0 map[string]any
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload0))
	assert.Equal(t, "balance.config-changed", emitted[0].DefinitionKey)
	assert.Equal(t, companionID, emitted[0].Subject, "first event must be for the COMPANION balance")
	assert.Equal(t, "overdraft_enabled", payload0["changeType"])
	assert.Equal(t, constant.OverdraftBalanceKey, payload0["key"])

	// Event 1: parent's config_changed{settings_updated}
	var payload1 map[string]any
	require.NoError(t, json.Unmarshal(emitted[1].Payload, &payload1))
	assert.Equal(t, "balance.config-changed", emitted[1].DefinitionKey)
	assert.Equal(t, id, emitted[1].Subject, "second event must be for the PARENT balance")
	assert.Equal(t, "settings_updated", payload1["changeType"])
}

// TestUpdateBalance_NoopEmitterDoesNotPanic exercises the
// disabled-flag path.
func TestUpdateBalance_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := defaultUpdateBalanceStreamingFixture()
	uc := newUpdateBalanceStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixture)

	allowSending := false
	update := mmodel.UpdateBalance{AllowSending: &allowSending}
	_, err := uc.Update(context.Background(), uuid.New(), uuid.New(), uuid.MustParse(fixture.updated.ID), update)
	require.NoError(t, err)
}

// TestUpdateBalance_EmitFailureDoesNotFailRequest verifies IMPORTANT
// posture.
func TestUpdateBalance_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := defaultUpdateBalanceStreamingFixture()
	uc := newUpdateBalanceStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixture)

	allowSending := false
	update := mmodel.UpdateBalance{AllowSending: &allowSending}
	out, err := uc.Update(context.Background(), uuid.New(), uuid.New(), uuid.MustParse(fixture.updated.ID), update)
	require.NoError(t, err, "Emit failure must NOT fail the request")
	require.NotNil(t, out)
}

// TestUpdateBalance_NilStreamingDoesNotPanic exercises the partial-wiring path.
func TestUpdateBalance_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := defaultUpdateBalanceStreamingFixture()
	uc := newUpdateBalanceStreamingTestUseCase(t, ctrl, nil, fixture)

	allowSending := false
	update := mmodel.UpdateBalance{AllowSending: &allowSending}
	_, err := uc.Update(context.Background(), uuid.New(), uuid.New(), uuid.MustParse(fixture.updated.ID), update)
	require.NoError(t, err)
}
