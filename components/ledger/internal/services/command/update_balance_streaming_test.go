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

	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
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
		Return(fixture.updated, false, nil).
		AnyTimes()

	// The configuration write succeeds and the overlay finds nothing cached.
	mockRedis := txRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		AnyTimes()
	expectConfigurationWriteSucceeds(mockRedis)

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
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "balance", "config_changed")

	evt := emitted[0]
	assert.Equal(t, "balance.config_changed", evt.DefinitionKey)
	assert.Equal(t, fixture.updated.ID, evt.Subject)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))
	assert.Equal(t, "settings_updated", payload["changeType"])
	assert.Equal(t, fixture.updated.ID, payload["id"])
	assert.Equal(t, fixture.updated.AccountID, payload["accountId"])
	assert.Equal(t, false, payload["allowSending"])
}

// TestUpdateBalance_EmitsTwoEventsOnOverdraftTransition verifies that a
// PATCH whose UPDATE switches AllowOverdraft false->true emits TWO
// config_changed events: first for the companion (overdraft_enabled)
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
	mockBalanceRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&updated, true, nil).AnyTimes()

	mockRedis := txRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	expectConfigurationWriteSucceeds(mockRedis)

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
	assert.Equal(t, "balance.config_changed", emitted[0].DefinitionKey)
	assert.Equal(t, companionID, emitted[0].Subject, "first event must be for the COMPANION balance")
	assert.Equal(t, "overdraft_enabled", payload0["changeType"])
	assert.Equal(t, constant.OverdraftBalanceKey, payload0["key"])

	// Event 1: parent's config_changed{settings_updated}
	var payload1 map[string]any
	require.NoError(t, json.Unmarshal(emitted[1].Payload, &payload1))
	assert.Equal(t, "balance.config_changed", emitted[1].DefinitionKey)
	assert.Equal(t, id, emitted[1].Subject, "second event must be for the PARENT balance")
	assert.Equal(t, "settings_updated", payload1["changeType"])
}

// expectConfigurationWriteSucceeds lets every configuration key write and blob
// copy succeed.
func expectConfigurationWriteSucceeds(mockRedis *txRedis.MockRedisRepository) {
	mockRedis.EXPECT().
		WriteBalanceConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ string, config accounting.BalanceConfig) (accounting.BalanceConfig, txRedis.ConfigWriteOutcome, error) {
			return config, txRedis.ConfigWritten, nil
		}).
		AnyTimes()
	mockRedis.EXPECT().
		CopyBalanceConfigToBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(txRedis.BlobCopyWritten, nil).
		AnyTimes()
}

// TestUpdateBalance_ConfigurationWriteFailureStillEmits verifies that the
// events announce the persisted configuration: they go out right after the
// commit, so a cache failure that fails the PATCH does not lose them. The
// client's retry commits nothing new and publishes settings_updated again,
// which is a repeated announcement of the same row, never a lost one.
func TestUpdateBalance_ConfigurationWriteFailureStillEmits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	fixture := defaultUpdateBalanceStreamingFixture()

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fixture.current, nil).
		Times(1)
	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fixture.updated, false, nil).
		Times(1)

	mockRedis := txRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().
		WriteBalanceConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(accounting.BalanceConfig{}, txRedis.ConfigWriteOutcome(0), errors.New("redis unavailable")).
		Times(1)

	uc := &UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedis,
		Streaming:            mockEmitter,
	}

	allowSending := false
	update := mmodel.UpdateBalance{AllowSending: &allowSending}

	out, err := uc.Update(context.Background(), uuid.New(), uuid.New(), uuid.MustParse(fixture.updated.ID), update)
	require.Error(t, err)
	assert.Nil(t, out)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "the persisted configuration is announced even though the PATCH fails")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, "settings_updated", payload["changeType"])
}

// TestUpdateBalance_EventsGoOutBeforeTheConfigurationWrite pins the ordering
// on the happy path: by the time the configuration key is written, the events
// announcing the commit are already out.
func TestUpdateBalance_EventsGoOutBeforeTheConfigurationWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	fixture := defaultUpdateBalanceStreamingFixture()

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fixture.current, nil).
		Times(1)
	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fixture.updated, false, nil).
		Times(1)

	var eventsAtWrite int

	mockRedis := txRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().
		WriteBalanceConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ string, config accounting.BalanceConfig) (accounting.BalanceConfig, txRedis.ConfigWriteOutcome, error) {
			eventsAtWrite = len(mockEmitter.Events())

			return config, txRedis.ConfigWritten, nil
		}).
		Times(1)
	mockRedis.EXPECT().
		CopyBalanceConfigToBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(txRedis.BlobCopyAlreadyCurrent, nil).
		Times(1)
	mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()

	uc := &UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedis,
		Streaming:            mockEmitter,
	}

	allowSending := false
	update := mmodel.UpdateBalance{AllowSending: &allowSending}

	out, err := uc.Update(context.Background(), uuid.New(), uuid.New(), uuid.MustParse(fixture.updated.ID), update)
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, 1, eventsAtWrite, "the event is out before the configuration key is written")
	require.Len(t, mockEmitter.Events(), 1)
}

// TestUpdateBalance_OverdraftEnabledBelongsToTheTransition pins who announces
// the overdraft switching on: the request whose UPDATE made the false->true
// transition of the parent, with the companion readied before that UPDATE —
// whether this request created it, found it (a retry after a failure, or a
// current read already showing the overdraft on while another PATCH switched it
// off in between), or lost the race creating it. A request whose UPDATE did not
// make the transition announces only settings_updated, even if it created the
// companion.
func TestUpdateBalance_OverdraftEnabledBelongsToTheTransition(t *testing.T) {
	cases := []struct {
		name            string
		currentAllows   bool
		companionExists bool
		transition      bool
		wantEvents      []string
	}{
		{name: "created companion, transition", transition: true, wantEvents: []string{"overdraft_enabled", "settings_updated"}},
		{name: "existing companion on retry, transition", companionExists: true, transition: true, wantEvents: []string{"overdraft_enabled", "settings_updated"}},
		{name: "current read shows overdraft on, transition", currentAllows: true, companionExists: true, transition: true, wantEvents: []string{"overdraft_enabled", "settings_updated"}},
		{name: "created companion, no transition", wantEvents: []string{"settings_updated"}},
		{name: "existing companion, no transition", companionExists: true, currentAllows: true, wantEvents: []string{"settings_updated"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockEmitter := pkgStreaming.NewMockEmitter()

			fixture := defaultUpdateBalanceStreamingFixture()
			fixture.current.Settings = &mmodel.BalanceSettings{AllowOverdraft: tc.currentAllows}
			fixture.updated.Settings = &mmodel.BalanceSettings{AllowOverdraft: true}

			companion := &mmodel.Balance{
				ID: uuid.New().String(), AccountID: fixture.current.AccountID, Alias: fixture.current.Alias,
				Key: constant.OverdraftBalanceKey, AssetCode: "USD", Direction: constant.DirectionDebit,
				Settings: &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal},
			}

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockBalanceRepo.EXPECT().Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fixture.current, nil).Times(1)

			readied := mockBalanceRepo.EXPECT().
				FindByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), uuid.MustParse(fixture.current.AccountID), constant.OverdraftBalanceKey).
				Times(1)

			if tc.companionExists {
				readied.Return(companion, nil)
			} else {
				readied.Return(nil, pkg.EntityNotFoundError{EntityType: constant.EntityBalance})
				mockBalanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(companion, nil).Times(1)
			}

			mockBalanceRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(fixture.updated, tc.transition, nil).After(readied).Times(1)

			mockRedis := txRedis.NewMockRedisRepository(ctrl)
			mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
			expectConfigurationWriteSucceeds(mockRedis)

			uc := &UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedis, Streaming: mockEmitter}

			update := mmodel.UpdateBalance{Settings: &mmodel.BalanceSettings{AllowOverdraft: true}}

			_, err := uc.Update(context.Background(), uuid.New(), uuid.New(), uuid.MustParse(fixture.updated.ID), update)
			require.NoError(t, err)

			emitted := mockEmitter.Events()
			got := make([]string, 0, len(emitted))

			for _, evt := range emitted {
				var payload map[string]any
				require.NoError(t, json.Unmarshal(evt.Payload, &payload))

				got = append(got, payload["changeType"].(string))

				if payload["changeType"] == "overdraft_enabled" {
					assert.Equal(t, companion.ID, evt.Subject, "overdraft_enabled carries the readied companion")
				}
			}

			assert.Equal(t, tc.wantEvents, got)
		})
	}
}

// TestUpdateBalance_CompanionFailureLeavesTheParentUntouched verifies that the
// companion is readied before the parent's UPDATE: when reading or creating it
// fails, the parent is never updated and nothing is announced.
func TestUpdateBalance_CompanionFailureLeavesTheParentUntouched(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockEmitter := pkgStreaming.NewMockEmitter()
	fixture := defaultUpdateBalanceStreamingFixture()
	fixture.current.Settings = &mmodel.BalanceSettings{AllowOverdraft: true}

	lookupErr := errors.New("connection reset")

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fixture.current, nil).Times(1)
	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.OverdraftBalanceKey).
		Return(nil, lookupErr).
		Times(1)
	mockBalanceRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	uc := &UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: txRedis.NewMockRedisRepository(ctrl), Streaming: mockEmitter}

	update := mmodel.UpdateBalance{Settings: &mmodel.BalanceSettings{AllowOverdraft: true}}

	_, err := uc.Update(context.Background(), uuid.New(), uuid.New(), uuid.MustParse(fixture.updated.ID), update)
	require.ErrorIs(t, err, lookupErr)
	assert.Empty(t, mockEmitter.Events())
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
