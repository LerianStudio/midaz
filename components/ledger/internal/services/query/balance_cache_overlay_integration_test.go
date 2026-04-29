// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// These tests exercise the cache-overlay completeness contract end-to-end at
// every query method that overlays the Redis cache onto a PostgreSQL-sourced
// balance: GetAllBalancesByAccountID, GetAllBalances, GetAllBalancesByAlias,
// and GetBalanceByID. The goal is to catch the original bug class (cache
// fields silently lost on the response path) regardless of whether a future
// refactor inlines, splits, or relocates the overlay helper.

// makeOverdraftCacheJSON builds a Redis-shape balance payload with the
// overdraft fields populated. Mirrors the CamelCase-aware shape used by the
// Lua atomic script and consumer.redis.go.
func makeOverdraftCacheJSON(t *testing.T, version int64, overdraftUsed string, allowOverdraft, limitEnabled int, overdraftLimit, scope, direction string) string {
	t.Helper()

	c := mmodel.BalanceRedis{
		Available:             decimal.Zero,
		OnHold:                decimal.Zero,
		Version:               version,
		Direction:             direction,
		OverdraftUsed:         overdraftUsed,
		AllowOverdraft:        allowOverdraft,
		OverdraftLimitEnabled: limitEnabled,
		OverdraftLimit:        overdraftLimit,
		BalanceScope:          scope,
	}
	data, err := json.Marshal(c)
	require.NoError(t, err)

	return string(data)
}

// makeStaleBalance returns a PG-sourced Balance carrying pre-transaction
// values. After the cache overlay runs it must be visibly newer.
func makeStaleBalance(alias, key string) *mmodel.Balance {
	return &mmodel.Balance{
		ID:            uuid.New().String(),
		Alias:         alias,
		Key:           key,
		AssetCode:     "BRL",
		Available:     decimal.NewFromInt(50), // pre-transaction
		OnHold:        decimal.Zero,
		Version:       1,
		Direction:     "credit",
		OverdraftUsed: decimal.Zero, // stale: PG hasn't seen the transaction
		Settings: &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        ptrString("500"),
		},
	}
}

func ptrString(s string) *string { return &s }

// --- GetAllBalancesByAccountID ---------------------------------------------

func TestGetAllBalancesByAccountID_CacheOverlay_PropagatesOverdraftFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	filter := http.QueryHeader{
		Limit:        10,
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{Next: "n", Prev: "p"}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	bal := makeStaleBalance("@user", "default")
	mockBalanceRepo.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
		Return([]*mmodel.Balance{bal}, mockCur, nil).
		Times(1)

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
	// Cache is post-transaction: deficit of 130, full overdraft state.
	payload := makeOverdraftCacheJSON(t, 7, "130.00", 1, 1, "500", mmodel.BalanceScopeTransactional, "credit")
	mockRedisRepo.
		EXPECT().
		MGet(gomock.Any(), []string{cacheKey}).
		Return(map[string]string{cacheKey: payload}, nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
	res, _, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

	require.NoError(t, err)
	require.Len(t, res, 1)

	assert.Equal(t, int64(7), res[0].Version, "Version must overlay from cache")
	assert.Equal(t, "credit", res[0].Direction, "Direction must overlay from cache")
	assert.True(t, res[0].OverdraftUsed.Equal(decimal.NewFromInt(130)),
		"OverdraftUsed must overlay from cache")
	require.NotNil(t, res[0].Settings)
	assert.True(t, res[0].Settings.AllowOverdraft)
	assert.True(t, res[0].Settings.OverdraftLimitEnabled)
	require.NotNil(t, res[0].Settings.OverdraftLimit)
	assert.Equal(t, "500", *res[0].Settings.OverdraftLimit)
}

func TestGetAllBalancesByAccountID_CacheMiss_PreservesPGOverdraftFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	filter := http.QueryHeader{
		Limit:     10,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   time.Now(),
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	bal := &mmodel.Balance{
		ID:            uuid.New().String(),
		Alias:         "@user",
		Key:           "default",
		Available:     decimal.NewFromInt(75),
		OnHold:        decimal.Zero,
		Version:       3,
		Direction:     "credit",
		OverdraftUsed: decimal.NewFromInt(25),
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        ptrString("100"),
			BalanceScope:          mmodel.BalanceScopeTransactional,
		},
	}

	mockBalanceRepo.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
		Return([]*mmodel.Balance{bal}, libHTTP.CursorPagination{}, nil).
		Times(1)

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
	mockRedisRepo.
		EXPECT().
		MGet(gomock.Any(), []string{cacheKey}).
		Return(map[string]string{}, nil). // cache miss
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
	res, _, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

	require.NoError(t, err)
	require.Len(t, res, 1)

	// All fields must come from PG since the cache had nothing to say.
	assert.True(t, res[0].Available.Equal(decimal.NewFromInt(75)))
	assert.True(t, res[0].OverdraftUsed.Equal(decimal.NewFromInt(25)))
	assert.Equal(t, "credit", res[0].Direction)
	require.NotNil(t, res[0].Settings)
	require.NotNil(t, res[0].Settings.OverdraftLimit)
	assert.Equal(t, "100", *res[0].Settings.OverdraftLimit)
}

func TestGetAllBalancesByAccountID_MixedBatch_PerBalanceResolution(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	filter := http.QueryHeader{
		Limit:     10,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   time.Now(),
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	cached := makeStaleBalance("@user", "default")
	missing := makeStaleBalance("@user", "savings")

	mockBalanceRepo.
		EXPECT().
		ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
		Return([]*mmodel.Balance{cached, missing}, libHTTP.CursorPagination{}, nil).
		Times(1)

	keyCached := utils.BalanceInternalKey(organizationID, ledgerID, cached.Alias+"#"+cached.Key)
	keyMissing := utils.BalanceInternalKey(organizationID, ledgerID, missing.Alias+"#"+missing.Key)
	payload := makeOverdraftCacheJSON(t, 9, "200", 1, 1, "500", mmodel.BalanceScopeTransactional, "credit")

	mockRedisRepo.
		EXPECT().
		MGet(gomock.Any(), []string{keyCached, keyMissing}).
		Return(map[string]string{keyCached: payload}, nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
	res, _, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

	require.NoError(t, err)
	require.Len(t, res, 2)

	// First balance: overlay applied
	assert.True(t, res[0].OverdraftUsed.Equal(decimal.NewFromInt(200)))
	assert.Equal(t, int64(9), res[0].Version)

	// Second balance: cache miss, PG values preserved
	assert.True(t, res[1].OverdraftUsed.Equal(decimal.Zero))
	assert.Equal(t, int64(1), res[1].Version)
}

// --- GetAllBalances --------------------------------------------------------

func TestGetAllBalances_CacheOverlay_PropagatesOverdraftFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	filter := http.QueryHeader{
		Limit:     10,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   time.Now(),
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	bal := makeStaleBalance("@user", "default")
	mockBalanceRepo.
		EXPECT().
		ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return([]*mmodel.Balance{bal}, libHTTP.CursorPagination{}, nil).
		Times(1)

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
	payload := makeOverdraftCacheJSON(t, 4, "75.00", 1, 1, "500", mmodel.BalanceScopeTransactional, "credit")
	mockRedisRepo.
		EXPECT().
		MGet(gomock.Any(), []string{cacheKey}).
		Return(map[string]string{cacheKey: payload}, nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
	res, _, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

	require.NoError(t, err)
	require.Len(t, res, 1)

	assert.Equal(t, int64(4), res[0].Version)
	assert.True(t, res[0].OverdraftUsed.Equal(decimal.NewFromInt(75)))
	assert.Equal(t, "credit", res[0].Direction)
	require.NotNil(t, res[0].Settings)
	assert.True(t, res[0].Settings.AllowOverdraft)
}

// --- GetAllBalancesByAlias -------------------------------------------------

func TestGetAllBalancesByAlias_CacheOverlay_PropagatesOverdraftFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	bal := makeStaleBalance("@user", "default")
	mockBalanceRepo.
		EXPECT().
		ListByAliases(gomock.Any(), organizationID, ledgerID, []string{bal.Alias}).
		Return([]*mmodel.Balance{bal}, nil).
		Times(1)

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
	payload := makeOverdraftCacheJSON(t, 11, "33", 1, 1, "500", mmodel.BalanceScopeTransactional, "credit")
	mockRedisRepo.
		EXPECT().
		MGet(gomock.Any(), []string{cacheKey}).
		Return(map[string]string{cacheKey: payload}, nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
	res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, bal.Alias)

	require.NoError(t, err)
	require.Len(t, res, 1)

	assert.Equal(t, int64(11), res[0].Version)
	assert.True(t, res[0].OverdraftUsed.Equal(decimal.NewFromInt(33)))
	assert.Equal(t, "credit", res[0].Direction)
}

// --- GetBalanceByID --------------------------------------------------------

// TestGetBalanceByID_CacheOverlay_PropagatesOverdraftFields covers the
// single-balance read path used by GET /v1/.../balances/{id}. It must
// propagate OverdraftUsed, Direction, and Settings from the cache
// identically to the list endpoints; otherwise the response carries stale
// PG values until the write-behind worker syncs, and the derived
// position.available field on overdrafted accounts surfaces as zero
// instead of negative.
func TestGetBalanceByID_CacheOverlay_PropagatesOverdraftFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	bal := makeStaleBalance("@user", "default")
	bal.ID = balanceID.String()

	mockBalanceRepo.
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(bal, nil).
		Times(1)

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
	// Cache is post-transaction: source overdrafted by 100 (Available=0,
	// OverdraftUsed=100). This is the exact scenario that drove the position
	// E2E test failure.
	payload := makeOverdraftCacheJSON(t, 5, "100", 1, 1, "500", mmodel.BalanceScopeTransactional, "credit")
	mockRedisRepo.
		EXPECT().
		Get(gomock.Any(), cacheKey).
		Return(payload, nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
	res, err := uc.GetBalanceByID(context.TODO(), organizationID, ledgerID, balanceID)

	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, int64(5), res.Version, "Version must overlay from cache")
	assert.Equal(t, "credit", res.Direction, "Direction must overlay from cache")
	assert.True(t, res.OverdraftUsed.Equal(decimal.NewFromInt(100)),
		"OverdraftUsed must overlay from cache (was the position-test gap)")
	require.NotNil(t, res.Settings)
	assert.True(t, res.Settings.AllowOverdraft)
	assert.True(t, res.Settings.OverdraftLimitEnabled)
	require.NotNil(t, res.Settings.OverdraftLimit)
	assert.Equal(t, "500", *res.Settings.OverdraftLimit)
}

// TestGetBalanceByID_PositionField_NegativeOnOverdrafted exercises the full
// stack from the GetBalanceByID return value through Balance.MarshalJSON's
// Position computation, asserting the wire shape carries position.available
// = -100 when Available=0 and OverdraftUsed=100. Pins the negative-position
// contract on overdrafted accounts.
func TestGetBalanceByID_PositionField_NegativeOnOverdrafted(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// PG view: stale, claims Available=50, OverdraftUsed=0 (pre-transaction).
	bal := makeStaleBalance("@user", "default")
	bal.ID = balanceID.String()

	mockBalanceRepo.
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(bal, nil).
		Times(1)

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
	// Cache view: post-transaction, Available=0 OverdraftUsed=100.
	cached := mmodel.BalanceRedis{
		Available:             decimal.Zero,
		OnHold:                decimal.Zero,
		Version:               5,
		Direction:             "credit",
		OverdraftUsed:         "100",
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 1,
		OverdraftLimit:        "500",
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}
	data, _ := json.Marshal(cached)

	mockRedisRepo.
		EXPECT().
		Get(gomock.Any(), cacheKey).
		Return(string(data), nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
	res, err := uc.GetBalanceByID(context.TODO(), organizationID, ledgerID, balanceID)

	require.NoError(t, err)
	require.NotNil(t, res)

	// Marshal through Balance.MarshalJSON to exercise the full position
	// surface the way an HTTP client would receive it.
	wire, err := json.Marshal(res)
	require.NoError(t, err)

	var envelope struct {
		Position struct {
			Available               string  `json:"available"`
			OnHold                  string  `json:"onHold"`
			OverdraftLimitAvailable *string `json:"overdraftLimitAvailable"`
		} `json:"position"`
	}
	require.NoError(t, json.Unmarshal(wire, &envelope))

	assert.Equal(t, "-100", envelope.Position.Available,
		"position.available must be Available − OverdraftUsed = 0 − 100 = -100")
	assert.Equal(t, "0", envelope.Position.OnHold)
	require.NotNil(t, envelope.Position.OverdraftLimitAvailable)
	assert.Equal(t, "400", *envelope.Position.OverdraftLimitAvailable,
		"position.overdraftLimitAvailable must be limit − used = 500 − 100 = 400")
}

// TestGetAllBalances_CachePresentButSettingsDefault_PreservesPGSettings is the
// regression guard for the Settings nil-safety contract: if the cache carries
// no divergent settings (e.g. a legacy entry written before the settings
// write-through path), the PG-sourced Settings must NOT be clobbered by an
// empty BalanceSettings struct.
func TestGetAllBalances_CachePresentButSettingsDefault_PreservesPGSettings(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	filter := http.QueryHeader{
		Limit:     10,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   time.Now(),
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	bal := makeStaleBalance("@user", "default")
	mockBalanceRepo.
		EXPECT().
		ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return([]*mmodel.Balance{bal}, libHTTP.CursorPagination{}, nil).
		Times(1)

	cacheKey := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
	// Cache reports fresh runtime numbers but ZERO divergent settings —
	// represents either a legacy entry or a balance where overdraft was
	// never enabled. Either way: PG Settings must stay intact.
	payload := makeOverdraftCacheJSON(t, 6, "0", 0, 0, "", "", "")
	mockRedisRepo.
		EXPECT().
		MGet(gomock.Any(), []string{cacheKey}).
		Return(map[string]string{cacheKey: payload}, nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
	res, _, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

	require.NoError(t, err)
	require.Len(t, res, 1)

	// Runtime fields overlaid from cache:
	assert.Equal(t, int64(6), res[0].Version)
	assert.True(t, res[0].OverdraftUsed.Equal(decimal.Zero))

	// Settings: PG value preserved (cache had no divergent data).
	require.NotNil(t, res[0].Settings, "PG Settings must not be nilled out by an empty cache entry")
	assert.True(t, res[0].Settings.AllowOverdraft)
	require.NotNil(t, res[0].Settings.OverdraftLimit)
	assert.Equal(t, "500", *res[0].Settings.OverdraftLimit)

	// Direction: cache empty, PG preserved.
	assert.Equal(t, "credit", res[0].Direction)
}
