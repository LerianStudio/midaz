// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// The seed guard exists because the balance row is written asynchronously: when the
// cached balance is lost while a delta is still pending, PostgreSQL is behind the
// operation trail and seeding from it forks the balance. These tests pin exactly when
// the guard acts, what it replaces, and what it leaves alone.

// Fixed identities — the guard keys everything by balance ID, so the IDs are part of
// the contract under test.
var (
	seedGuardOrgID     = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	seedGuardLedgerID  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	seedGuardAccountID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	seedGuardBalanceID = uuid.MustParse("44444444-4444-4444-4444-444444444444")
	seedGuardSecondID  = uuid.MustParse("55555555-5555-5555-5555-555555555555")
	seedGuardThirdID   = uuid.MustParse("66666666-6666-6666-6666-666666666666")
)

type seedGuardMocks struct {
	uc        *UseCase
	balance   *balance.MockRepository
	account   *account.MockRepository
	redis     *redis.MockRedisRepository
	operation *operation.MockRepository
	reader    *sdkmetric.ManualReader
}

// newSeedGuardUseCase wires a UseCase whose cache always misses, so every call under
// test reaches the seed path.
func newSeedGuardUseCase(t *testing.T, withMetrics bool) *seedGuardMocks {
	t.Helper()

	ctrl := gomock.NewController(t)

	mocks := &seedGuardMocks{
		balance:   balance.NewMockRepository(ctrl),
		account:   account.NewMockRepository(ctrl),
		redis:     redis.NewMockRedisRepository(ctrl),
		operation: operation.NewMockRepository(ctrl),
	}

	mocks.uc = &UseCase{
		BalanceRepo:          mocks.balance,
		AccountRepo:          mocks.account,
		TransactionRedisRepo: mocks.redis,
		OperationRepo:        mocks.operation,
	}

	if withMetrics {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

		t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

		factory, err := metrics.NewMetricsFactory(mp.Meter("seed-guard-test"), nil)
		require.NoError(t, err)

		mocks.uc.MetricsFactory = factory
		mocks.reader = reader
	}

	return mocks
}

// expectCacheMiss makes every alias miss the cache and returns the rows PostgreSQL holds.
func (m *seedGuardMocks) expectCacheMiss(aliases []string, rows []*mmodel.Balance) {
	for _, alias := range aliases {
		m.redis.EXPECT().
			Get(gomock.Any(), utils.BalanceInternalKey(seedGuardOrgID, seedGuardLedgerID, alias)).
			Return("", nil).
			Times(1)
	}

	m.balance.EXPECT().
		ListByAliasesWithKeys(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, aliases).
		Return(rows, nil).
		Times(1)

	if len(rows) > 0 {
		m.account.EXPECT().
			ListAccountsByIDs(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
			Return([]*mmodel.Account{}, nil).
			Times(1)
	}

	// The seed also coordinates with closing; these rows belong to an account that
	// was never closed, so the protection answers absence throughout.
	expectOpenAccountAdmission(m.redis, m.account)
}

// seedGuardRow is a balance row as PostgreSQL holds it, with identity fields set so a
// rebuild can be shown to leave them alone.
func seedGuardRow(id uuid.UUID, key string, available, onHold, overdraftUsed decimal.Decimal, version int64) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             id.String(),
		OrganizationID: seedGuardOrgID.String(),
		LedgerID:       seedGuardLedgerID.String(),
		AccountID:      seedGuardAccountID.String(),
		Alias:          "@alice",
		Key:            key,
		AssetCode:      "USD",
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      "credit",
		Available:      available,
		OnHold:         onHold,
		OverdraftUsed:  overdraftUsed,
		Version:        version,
		Settings:       &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeTransactional},
	}
}

// seedGuardHWM is the operation holding a balance's high-water mark.
func seedGuardHWM(available, onHold decimal.Decimal, version int64, overdraftAfter string) *operation.Operation {
	return &operation.Operation{
		BalanceAfter: operation.Balance{
			Available: &available,
			OnHold:    &onHold,
			Version:   &version,
		},
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "0",
			OverdraftUsedAfter:  overdraftAfter,
		},
	}
}

func TestSeedGuard_RowAtHighWaterMarkPassesThrough(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)
	row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey,
		decimal.NewFromInt(100), decimal.NewFromInt(10), decimal.NewFromInt(5), 2)

	mocks.expectCacheMiss([]string{"@alice#default"}, []*mmodel.Balance{row})
	mocks.operation.EXPECT().
		ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{
			seedGuardBalanceID.String(): seedGuardHWM(decimal.NewFromInt(999), decimal.Zero, 2, "42"),
		}, nil).
		Times(1)

	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@alice#default"})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, decimal.NewFromInt(100).Equal(balances[0].Available), "a row level with the trail must not be touched")
	assert.True(t, decimal.NewFromInt(10).Equal(balances[0].OnHold))
	assert.True(t, decimal.NewFromInt(5).Equal(balances[0].OverdraftUsed))
	assert.Equal(t, int64(2), balances[0].Version)
}

func TestSeedGuard_StaleRowIsRebuiltFromTheTrail(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, true)
	row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey,
		decimal.NewFromInt(100), decimal.Zero, decimal.Zero, 1)

	mocks.expectCacheMiss([]string{"@alice#default"}, []*mmodel.Balance{row})
	mocks.operation.EXPECT().
		ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{
			seedGuardBalanceID.String(): seedGuardHWM(decimal.NewFromInt(150), decimal.NewFromInt(30), 2, "20"),
		}, nil).
		Times(1)

	logs := &recordingLogger{}
	ctx := libObservability.ContextWithLogger(context.Background(), logs)

	balances, err := mocks.uc.GetBalances(ctx, seedGuardOrgID, seedGuardLedgerID, []string{"@alice#default"})

	require.NoError(t, err)
	require.Len(t, balances, 1)

	got := balances[0]
	assert.True(t, decimal.NewFromInt(150).Equal(got.Available), "the seed must carry the trail's available")
	assert.True(t, decimal.NewFromInt(30).Equal(got.OnHold))
	assert.True(t, decimal.NewFromInt(20).Equal(got.OverdraftUsed))
	assert.Equal(t, int64(2), got.Version)

	// Identity and configuration belong to the row, not to the operation.
	assert.Equal(t, seedGuardBalanceID.String(), got.ID)
	assert.Equal(t, seedGuardAccountID.String(), got.AccountID)
	assert.Equal(t, "@alice", got.Alias)
	assert.Equal(t, constant.DefaultBalanceKey, got.Key)
	assert.Equal(t, "USD", got.AssetCode)
	assert.Equal(t, "deposit", got.AccountType)
	assert.Equal(t, "credit", got.Direction)
	assert.True(t, got.AllowSending)
	assert.True(t, got.AllowReceiving)
	require.NotNil(t, got.Settings)
	assert.Equal(t, mmodel.BalanceScopeTransactional, got.Settings.BalanceScope)

	assert.Equal(t, int64(1), seedRebuiltCount(t, mocks.reader), "each rebuilt balance must be counted")

	// The warning has to say how far behind the row was, which means reporting the
	// version the row carried BEFORE the rebuild — reading it afterwards would print
	// the high-water mark twice and hide the size of the gap.
	warning := logs.entryWith(t, "Rebuilt stale balance seed from the operation trail")
	assert.Equal(t, seedGuardBalanceID.String(), warning.fields["balance_id"])
	assert.Equal(t, 1, warning.fields["row_version"], "row_version must be the pre-rebuild version")
	assert.Equal(t, 2, warning.fields["hwm_version"])
}

func TestSeedGuard_RowAheadOfTheTrailPassesThrough(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)
	row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey,
		decimal.NewFromInt(500), decimal.Zero, decimal.Zero, 9)

	mocks.expectCacheMiss([]string{"@alice#default"}, []*mmodel.Balance{row})
	mocks.operation.EXPECT().
		ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{
			seedGuardBalanceID.String(): seedGuardHWM(decimal.NewFromInt(150), decimal.Zero, 2, "0"),
		}, nil).
		Times(1)

	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@alice#default"})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, decimal.NewFromInt(500).Equal(balances[0].Available),
		"a completion that landed after the last operation is not a fork")
	assert.Equal(t, int64(9), balances[0].Version)
}

func TestSeedGuard_BalanceWithoutTrailPassesThrough(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)
	row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey,
		decimal.NewFromInt(100), decimal.Zero, decimal.Zero, 0)

	mocks.expectCacheMiss([]string{"@alice#default"}, []*mmodel.Balance{row})
	mocks.operation.EXPECT().
		ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{}, nil).
		Times(1)

	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@alice#default"})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, decimal.NewFromInt(100).Equal(balances[0].Available), "a brand-new account has nothing to rebuild from")
	assert.Equal(t, int64(0), balances[0].Version)
}

func TestSeedGuard_UnusableHighWaterMarkRejectsTheRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hwm  *operation.Operation
	}{
		{
			name: "missing after-value",
			hwm: &operation.Operation{
				BalanceAfter: operation.Balance{OnHold: decimalPtr(decimal.Zero), Version: int64Ptr(2)},
				Snapshot:     mmodel.OperationSnapshot{OverdraftUsedAfter: "0"},
			},
		},
		{
			name: "overdraft snapshot is not a decimal",
			hwm: &operation.Operation{
				BalanceAfter: operation.Balance{
					Available: decimalPtr(decimal.NewFromInt(150)),
					OnHold:    decimalPtr(decimal.Zero),
					Version:   int64Ptr(2),
				},
				Snapshot: mmodel.OperationSnapshot{OverdraftUsedAfter: "not-a-decimal"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mocks := newSeedGuardUseCase(t, false)
			row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey,
				decimal.NewFromInt(100), decimal.Zero, decimal.Zero, 1)

			mocks.expectCacheMiss([]string{"@alice#default"}, []*mmodel.Balance{row})
			mocks.operation.EXPECT().
				ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
				Return(map[string]*operation.Operation{seedGuardBalanceID.String(): tt.hwm}, nil).
				Times(1)

			balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@alice#default"})

			require.Error(t, err)
			assert.Nil(t, balances)

			unavailable, ok := err.(pkg.ServiceUnavailableError)
			require.True(t, ok, "an unusable high-water mark must refuse the read as 503, got %T", err)
			assert.Equal(t, constant.ErrBalanceSeedRebuildInconsistent.Error(), unavailable.Code)
		})
	}
}

func TestSeedGuard_MixedBatchUsesOneLookup(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)

	stale := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey,
		decimal.NewFromInt(100), decimal.Zero, decimal.Zero, 1)
	level := seedGuardRow(seedGuardSecondID, "savings",
		decimal.NewFromInt(200), decimal.Zero, decimal.Zero, 3)
	untracked := seedGuardRow(seedGuardThirdID, "fresh",
		decimal.NewFromInt(300), decimal.Zero, decimal.Zero, 0)

	aliases := []string{"@alice#default", "@alice#savings", "@alice#fresh"}
	mocks.expectCacheMiss(aliases, []*mmodel.Balance{stale, level, untracked})

	mocks.operation.EXPECT().
		ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, refs []operation.BalanceHWMRef) (map[string]*operation.Operation, error) {
			assert.Len(t, refs, 3, "every balance of the batch must ride the same lookup")

			return map[string]*operation.Operation{
				seedGuardBalanceID.String(): seedGuardHWM(decimal.NewFromInt(150), decimal.Zero, 2, "0"),
				seedGuardSecondID.String():  seedGuardHWM(decimal.NewFromInt(999), decimal.Zero, 3, "0"),
			}, nil
		}).
		Times(1)

	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, aliases)

	require.NoError(t, err)
	require.Len(t, balances, 3)

	byID := make(map[string]*mmodel.Balance, len(balances))
	for _, b := range balances {
		byID[b.ID] = b
	}

	assert.True(t, decimal.NewFromInt(150).Equal(byID[seedGuardBalanceID.String()].Available), "the stale row must be rebuilt")
	assert.True(t, decimal.NewFromInt(200).Equal(byID[seedGuardSecondID.String()].Available), "the level row must be untouched")
	assert.True(t, decimal.NewFromInt(300).Equal(byID[seedGuardThirdID.String()].Available), "the untracked row must be untouched")
}

func TestSeedGuard_CacheHitNeverReadsTheTrail(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)

	cached := mmodel.BalanceRedis{
		ID:             seedGuardBalanceID.String(),
		AccountID:      seedGuardAccountID.String(),
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		AssetCode:      "USD",
	}
	cachedJSON, err := json.Marshal(cached)
	require.NoError(t, err)

	mocks.redis.EXPECT().
		Get(gomock.Any(), utils.BalanceInternalKey(seedGuardOrgID, seedGuardLedgerID, "@alice#default")).
		Return(string(cachedJSON), nil).
		Times(1)

	// No ListLatestByBalances expectation: a cached balance is the live state, so
	// reading the trail would be pure overhead on the hot path.
	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@alice#default"})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, decimal.NewFromInt(100).Equal(balances[0].Available))
}

func TestSeedGuard_MissWithNoRowsNeverReadsTheTrail(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)
	mocks.expectCacheMiss([]string{"@ghost#default"}, []*mmodel.Balance{})

	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@ghost#default"})

	require.NoError(t, err)
	assert.Empty(t, balances)
}

func TestSeedGuard_OverdraftCompanionKeepsItsOwnOverdraftUsed(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)
	row := seedGuardRow(seedGuardBalanceID, constant.OverdraftBalanceKey,
		decimal.NewFromInt(100), decimal.Zero, decimal.NewFromInt(7), 1)

	mocks.expectCacheMiss([]string{"@alice#overdraft"}, []*mmodel.Balance{row})
	mocks.operation.EXPECT().
		ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{
			// The companion's operations carry the DEFAULT balance's overdraft snapshot.
			seedGuardBalanceID.String(): seedGuardHWM(decimal.NewFromInt(150), decimal.Zero, 2, "20"),
		}, nil).
		Times(1)

	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@alice#overdraft"})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, decimal.NewFromInt(150).Equal(balances[0].Available), "the money fields are still rebuilt")
	assert.Equal(t, int64(2), balances[0].Version)
	assert.True(t, decimal.NewFromInt(7).Equal(balances[0].OverdraftUsed),
		"the mirrored snapshot describes another balance, so the companion keeps its own value")
}

func TestSeedGuard_RebuildsWithoutMetricsFactory(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)
	require.Nil(t, mocks.uc.MetricsFactory)

	row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey,
		decimal.NewFromInt(100), decimal.Zero, decimal.Zero, 1)

	mocks.expectCacheMiss([]string{"@alice#default"}, []*mmodel.Balance{row})
	mocks.operation.EXPECT().
		ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{
			seedGuardBalanceID.String(): seedGuardHWM(decimal.NewFromInt(150), decimal.Zero, 2, "0"),
		}, nil).
		Times(1)

	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@alice#default"})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.True(t, decimal.NewFromInt(150).Equal(balances[0].Available), "telemetry being off must not change the read")
}

func TestSeedGuard_TrailLookupFailureFailsTheRead(t *testing.T) {
	t.Parallel()

	mocks := newSeedGuardUseCase(t, false)
	row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey,
		decimal.NewFromInt(100), decimal.Zero, decimal.Zero, 1)

	mocks.expectCacheMiss([]string{"@alice#default"}, []*mmodel.Balance{row})

	lookupErr := errors.New("operation trail unreachable")
	mocks.operation.EXPECT().
		ListLatestByBalances(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).
		Return(nil, lookupErr).
		Times(1)

	balances, err := mocks.uc.GetBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, []string{"@alice#default"})

	assert.ErrorIs(t, err, lookupErr,
		"without the trail the guard cannot tell a stale row from a fresh one, so the read must fail")
	assert.Nil(t, balances)
}

// seedRebuiltCount sums the rebuild counter across all label sets.
func seedRebuiltCount(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var total int64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != utils.BalanceSeedRebuilt.Name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "rebuild counter data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}

	return total
}

// recordingLogger captures what the use case logged. It satisfies log.Universal
// (one method), which is all ContextWithLogger needs.
type recordingLogger struct {
	mu      sync.Mutex
	entries []loggedEntry
}

type loggedEntry struct {
	msg    string
	fields map[string]any
}

func (r *recordingLogger) Log(_ context.Context, _ int, msg string, fields ...any) {
	entry := loggedEntry{msg: msg, fields: make(map[string]any, len(fields))}

	for _, f := range fields {
		if field, ok := f.(libLog.Field); ok {
			entry.fields[field.Key] = field.Value
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, entry)
}

// entryWith returns the single captured entry carrying msg.
func (r *recordingLogger) entryWith(t *testing.T, msg string) loggedEntry {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	var found []loggedEntry

	for _, e := range r.entries {
		if e.msg == msg {
			found = append(found, e)
		}
	}

	require.Len(t, found, 1, "expected exactly one %q log line, got %d", msg, len(found))

	return found[0]
}

func decimalPtr(d decimal.Decimal) *decimal.Decimal { return &d }

func int64Ptr(v int64) *int64 { return &v }
