// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// TestUpdateBalance tests the Update method with no Redis cached value
func TestUpdateBalance(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowSending := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: nil,
	}

	expectedBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@test",
		Key:            "default",
		AllowSending:   false,
		AllowReceiving: true,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Find is always called (scope guard requires the current balance).
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(expectedBalance, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(expectedBalance, false, nil).
		Times(1)

	// Redis returns empty (no cached value)
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		Times(1)

	// The committed row reaches the configuration key and the blob copy that
	// transactions obey.
	written := expectBalanceConfigurationWrite(t, mockRedisRepo, organizationID, ledgerID, "@test#default", redis.BlobCopyWritten)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedBalance.ID, result.ID)
	assert.False(t, result.AllowSending)
	assert.False(t, written.AllowSending)
	assert.True(t, written.AllowReceiving)
}

// TestUpdateBalance_RepoError tests the Update method when repository returns error
func TestUpdateBalance_RepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	errMSG := "errDatabaseItemNotFound"
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowSending := true
	allowReceiving := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: &allowReceiving,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	// Find is always called (scope guard requires the current balance).
	normalBalance := &mmodel.Balance{
		ID:    balanceID.String(),
		Alias: "@test",
		Key:   "default",
	}
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(normalBalance, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(nil, false, errors.New(errMSG))
	// Redis is NOT called when Update fails

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.Nil(t, result)
	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}

// TestUpdateBalance_RedisOverlay verifies that when Redis has cached balance values,
// they are overlayed onto the balance returned from the repository.
func TestUpdateBalance_RedisOverlay(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowSending := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending: &allowSending,
	}

	// Repository returns balance with initial values
	repoBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.NewFromInt(10),
		Version:        1,
		AllowSending:   false,
		AllowReceiving: true,
	}

	// Redis has fresher values that should be overlayed
	cachedBalance := mmodel.BalanceRedis{
		Available: decimal.NewFromInt(500),
		OnHold:    decimal.NewFromInt(50),
		Version:   5,
	}
	cachedJSON, err := json.Marshal(cachedBalance)
	require.NoError(t, err)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Find is always called (scope guard requires the current balance).
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(repoBalance, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(repoBalance, false, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(string(cachedJSON), nil).
		Times(1)

	// The configuration copy never touches the in-flight transactional
	// snapshot held in Redis (Available=500, OnHold=50, Version=5).
	expectBalanceConfigurationWrite(t, mockRedisRepo, organizationID, ledgerID, "@user1#default", redis.BlobCopyWritten)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Redis values should be overlayed
	assert.True(t, result.Available.Equal(decimal.NewFromInt(500)), "Available should be overlayed from Redis")
	assert.True(t, result.OnHold.Equal(decimal.NewFromInt(50)), "OnHold should be overlayed from Redis")
	assert.Equal(t, int64(5), result.Version, "Version should be overlayed from Redis")

	// Other fields should remain unchanged from repository
	assert.Equal(t, balanceID.String(), result.ID)
	assert.Equal(t, "@user1", result.Alias)
	assert.Equal(t, "USD", result.AssetCode)
	assert.False(t, result.AllowSending)
	assert.True(t, result.AllowReceiving)
}

// expectBalanceConfigurationWrite expects the configuration key write and then the
// blob copy of the balance cached under ref, both with the same configuration, and
// returns that configuration once the key write has seen it.
func expectBalanceConfigurationWrite(t *testing.T, redisRepo *redis.MockRedisRepository, organizationID, ledgerID uuid.UUID, ref string, copyOutcome redis.BlobCopyOutcome) *accounting.BalanceConfig {
	t.Helper()

	written := &accounting.BalanceConfig{}

	redisRepo.EXPECT().
		WriteBalanceConfig(gomock.Any(), organizationID, ledgerID, ref, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ string, config accounting.BalanceConfig) (accounting.BalanceConfig, redis.ConfigWriteOutcome, error) {
			*written = config

			return config, redis.ConfigWritten, nil
		}).
		Times(1)

	redisRepo.EXPECT().
		CopyBalanceConfigToBlob(gomock.Any(), organizationID, ledgerID, ref, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ string, config accounting.BalanceConfig) (redis.BlobCopyOutcome, error) {
			assert.Equal(t, *written, config, "the blob copy must carry the configuration written to the key, after it")

			return copyOutcome, nil
		}).
		Times(1)

	return written
}

// TestUpdateBalance_ConfigurationWrite_CarriesTheReturnedRow pins what reaches
// the configuration key: the whole row the UPDATE returned, with its version,
// under the composite alias#key the engine reads — not the PATCH payload. A
// settings PATCH therefore keeps the allow flags of the row, and an allow-flags
// PATCH keeps its settings.
func TestUpdateBalance_ConfigurationWrite_CarriesTheReturnedRow(t *testing.T) {
	t.Parallel()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowSending, allowReceiving := false, true

	returned := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "",
		AllowSending:   false,
		AllowReceiving: true,
		ConfigVersion:  7,
		Settings: &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        strPtr("250.50"),
		},
	}

	cases := map[string]mmodel.UpdateBalance{
		"allow flags": {AllowSending: &allowSending, AllowReceiving: &allowReceiving},
		"settings": {Settings: &mmodel.BalanceSettings{
			OverdraftLimitEnabled: true,
			OverdraftLimit:        strPtr("250.50"),
		}},
	}

	for name, update := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			current := *returned
			current.Settings = nil

			mockBalanceRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).Return(&current, nil).Times(1)
			mockBalanceRepo.EXPECT().Update(gomock.Any(), organizationID, ledgerID, balanceID, gomock.Any()).Return(returned, false, nil).Times(1)
			mockRedisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).Times(1)

			// A legacy row with an empty key is cached under the default key.
			written := expectBalanceConfigurationWrite(t, mockRedisRepo, organizationID, ledgerID, "@user1#default", redis.BlobCopyAlreadyCurrent)

			uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}

			_, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, update)
			require.NoError(t, err)

			assert.True(t, written.Equal(accounting.BalanceConfig{
				BalanceID:             balanceID,
				Version:               7,
				AllowSending:          false,
				AllowReceiving:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        decimal.RequireFromString("250.5"),
				BalanceScope:          mmodel.BalanceScopeTransactional,
			}), "written: %+v", *written)
		})
	}
}

// TestUpdateBalance_ConfigurationWrite_FailsClosed pins the fail-closed posture:
// PostgreSQL already committed, but answering success while the configuration key
// or the blob copy may still hold the previous configuration would let
// transactions keep obeying it. Every Redis failure, and a copy still unconfirmed
// after its compare-and-set attempts, fails the PATCH; the repository error
// surfaces unwrapped. A blob that is absent, already current or belongs to
// another balance (one deleted and recreated under the same alias#key) leaves
// nothing to confirm.
func TestUpdateBalance_ConfigurationWrite_FailsClosed(t *testing.T) {
	t.Parallel()

	errRedis := errors.New("redis connection refused")
	allowSending := false

	// An empty PATCH still writes the row's configuration and confirms its copy, so
	// a cache failure fails it too; a client that sends one may retry it safely.
	payloads := map[string]mmodel.UpdateBalance{
		"allow flags": {AllowSending: &allowSending},
		"settings":    {Settings: &mmodel.BalanceSettings{OverdraftLimitEnabled: true, OverdraftLimit: strPtr("10")}},
		"empty":       {},
	}

	// counted is the outcome the blob copy counter records, empty when no copy ran.
	cases := []struct {
		name        string
		writeErr    error
		copyOutcome redis.BlobCopyOutcome
		copyErr     error
		wantErr     error
		counted     string
	}{
		{name: "key write fails", writeErr: errRedis, wantErr: errRedis},
		{name: "blob copy fails", copyErr: errRedis, wantErr: errRedis, counted: "failed"},
		{name: "blob copy unconfirmed", copyOutcome: redis.BlobCopyConflict, wantErr: errConfigurationCopyNotConfirmed, counted: "conflict"},
		{name: "blob copy written", copyOutcome: redis.BlobCopyWritten, counted: "written"},
		{name: "blob already current", copyOutcome: redis.BlobCopyAlreadyCurrent, counted: "already_current"},
		{name: "blob absent", copyOutcome: redis.BlobCopyBlobAbsent, counted: "blob_absent"},
		{name: "blob of another balance", copyOutcome: redis.BlobCopyOfAnotherBalance, counted: "of_another_balance"},
	}

	for payloadName, update := range payloads {
		for _, tc := range cases {
			t.Run(payloadName+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				organizationID := uuid.Must(libCommons.GenerateUUIDv7())
				ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
				balanceID := uuid.Must(libCommons.GenerateUUIDv7())

				row := &mmodel.Balance{ID: balanceID.String(), Alias: "@user1", Key: "default", AllowReceiving: true, ConfigVersion: 3}

				ctrl := gomock.NewController(t)
				mockBalanceRepo := balance.NewMockRepository(ctrl)
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)

				mockBalanceRepo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).Return(row, nil).Times(1)
				mockBalanceRepo.EXPECT().Update(gomock.Any(), organizationID, ledgerID, balanceID, update).Return(row, false, nil).Times(1)

				mockRedisRepo.EXPECT().
					WriteBalanceConfig(gomock.Any(), organizationID, ledgerID, "@user1#default", gomock.Any()).
					Return(accounting.BalanceConfig{}, redis.ConfigWritten, tc.writeErr).
					Times(1)

				copyCalls := 1
				if tc.writeErr != nil {
					copyCalls = 0
				}

				mockRedisRepo.EXPECT().
					CopyBalanceConfigToBlob(gomock.Any(), organizationID, ledgerID, "@user1#default", gomock.Any()).
					Return(tc.copyOutcome, tc.copyErr).
					Times(copyCalls)

				// The overlay read sits after the gate.
				overlayCalls := 1
				if tc.wantErr != nil {
					overlayCalls = 0
				}

				mockRedisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).Times(overlayCalls)

				reader, factory := newReaderFactory(t)
				uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo, MetricsFactory: factory}

				result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, update)

				wantCounts := map[string]int64{}
				if tc.counted != "" {
					wantCounts["balance/"+tc.counted] = 1
				}

				assert.Equal(t, wantCounts, collectBlobCopyCounters(t, reader))

				if tc.wantErr == nil {
					require.NoError(t, err)
					require.NotNil(t, result)

					return
				}

				require.Error(t, err, "the PATCH must fail")
				assert.Nil(t, result)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.False(t, pkg.IsBusinessError(err), "a cache failure is a technical error")

				if tc.writeErr != nil || tc.copyErr != nil {
					assert.Equal(t, errRedis.Error(), err.Error(), "the repository error must surface unwrapped")
				}
			})
		}
	}
}

func TestUpdateBalances_PrimaryPath_UsesAfterDirectly(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balancesBefore := []*mmodel.Balance{
		{
			ID:        "bal-1",
			Alias:     "@alice",
			Available: decimal.NewFromInt(1000),
			OnHold:    decimal.NewFromInt(0),
			Version:   1,
		},
		{
			ID:        "bal-2",
			Alias:     "@bob",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.NewFromInt(0),
			Version:   3,
		},
	}

	balancesAfter := []*mmodel.Balance{
		{
			Alias:     "@alice",
			Available: decimal.NewFromInt(900),
			OnHold:    decimal.NewFromInt(0),
			Version:   2,
		},
		{
			Alias:     "@bob",
			Available: decimal.NewFromInt(600),
			OnHold:    decimal.NewFromInt(0),
			Version:   4,
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []*mmodel.Balance) error {
			require.Len(t, balances, 2)

			// Verify ID comes from BEFORE, values from AFTER
			assert.Equal(t, "bal-1", balances[0].ID)
			assert.Equal(t, "@alice", balances[0].Alias)
			assert.True(t, balances[0].Available.Equal(decimal.NewFromInt(900)))
			assert.Equal(t, int64(2), balances[0].Version)

			assert.Equal(t, "bal-2", balances[1].ID)
			assert.Equal(t, "@bob", balances[1].Alias)
			assert.True(t, balances[1].Available.Equal(decimal.NewFromInt(600)))
			assert.Equal(t, int64(4), balances[1].Version)

			return nil
		}).Times(1)

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	validate := mtransaction.Responses{}

	err := uc.UpdateBalances(context.TODO(), organizationID, ledgerID, validate, balancesBefore, balancesAfter)

	assert.NoError(t, err)
}

func TestUpdateBalances_FallbackPath_NilAfter(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balancesBefore := []*mmodel.Balance{
		{
			ID:          "bal-1",
			Alias:       "@alice",
			Available:   decimal.NewFromInt(1000),
			OnHold:      decimal.NewFromInt(0),
			Version:     1,
			AccountType: "deposit",
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []*mmodel.Balance) error {
			require.Len(t, balances, 1)

			// Fallback: OperateBalances recalculates, version is BEFORE+1
			assert.Equal(t, "bal-1", balances[0].ID)
			assert.True(t, balances[0].Available.Equal(decimal.NewFromInt(900)))
			assert.Equal(t, int64(2), balances[0].Version)

			return nil
		}).Times(1)

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	validate := mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			"@alice": {
				Value:           decimal.NewFromInt(100),
				Operation:       "DEBIT",
				TransactionType: "CREATED",
			},
		},
	}

	// nil balancesAfter triggers fallback
	err := uc.UpdateBalances(context.TODO(), organizationID, ledgerID, validate, balancesBefore, nil)

	assert.NoError(t, err)
}

func TestUpdateBalances_PrimaryPath_FailsOnMissingAlias(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balancesBefore := []*mmodel.Balance{
		{ID: "bal-1", Alias: "@alice"},
		{ID: "bal-2", Alias: "@bob"},
	}

	// Only @alice has AFTER state; incomplete payload must fail closed.
	balancesAfter := []*mmodel.Balance{
		{Alias: "@alice", Available: decimal.NewFromInt(900), OnHold: decimal.NewFromInt(0), Version: 2},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	uc := UseCase{BalanceRepo: mockBalanceRepo}

	err := uc.UpdateBalances(context.TODO(), organizationID, ledgerID, mtransaction.Responses{}, balancesBefore, balancesAfter)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing AFTER state for alias @bob")
}

func TestUpdateBalances_BalancesUpdateError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balancesBefore := []*mmodel.Balance{
		{ID: "bal-1", Alias: "@alice"},
	}
	balancesAfter := []*mmodel.Balance{
		{Alias: "@alice", Available: decimal.NewFromInt(900), Version: 2},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(errors.New("database connection error")).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo}

	err := uc.UpdateBalances(context.TODO(), organizationID, ledgerID, mtransaction.Responses{}, balancesBefore, balancesAfter)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}
