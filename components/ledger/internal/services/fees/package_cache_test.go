// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// errCacheTest is a sentinel cache-read failure used to exercise the fallback path.
var errCacheTest = errors.New("cache unavailable")

// cacheTestLogger returns the no-op logger lib-observability hands back for a
// bare context, so the direct-helper tests can call methods that take a logger.
func cacheTestLogger() libLog.Logger {
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	return logger
}

// fakePackageCache is an in-memory PackageCache test double that records the
// number of GetBytes / SetBytes / Del calls so the cache-aside behavior can be
// asserted without a live Redis. A nil store entry is treated as a miss.
type fakePackageCache struct {
	store map[string][]byte

	getCalls int
	setCalls int
	delCalls int

	// getErr, when set, is returned by GetBytes to exercise the fallback path.
	getErr error
}

func newFakePackageCache() *fakePackageCache {
	return &fakePackageCache{store: make(map[string][]byte)}
}

func (f *fakePackageCache) GetBytes(_ context.Context, key string) ([]byte, error) {
	f.getCalls++

	if f.getErr != nil {
		return nil, f.getErr
	}

	v, ok := f.store[key]
	if !ok {
		return nil, redis.Nil
	}

	return v, nil
}

func (f *fakePackageCache) SetBytes(_ context.Context, key string, value []byte, _ time.Duration) error {
	f.setCalls++
	f.store[key] = value

	return nil
}

func (f *fakePackageCache) Del(_ context.Context, key string) error {
	f.delCalls++
	delete(f.store, key)

	return nil
}

var _ PackageCache = (*fakePackageCache)(nil)

func cacheTestFeeInput(ledgerID uuid.UUID) *model.FeeCalculate {
	return &model.FeeCalculate{
		LedgerID: ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}
}

// TestCalculateFee_Cache_MissPopulatesSentinel proves that on a cache miss with
// zero packages in Mongo, the package set is fetched once and the NOT_FOUND
// sentinel is written, so the zero-package tenant is cached.
func TestCalculateFee_Cache_MissPopulatesSentinel(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := pack.NewMockRepository(ctrl)
	cache := newFakePackageCache()

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Mongo returns zero packages — queried exactly once.
	mockRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{}, nil).
		Times(1)

	uc := &UseCase{packageRepo: mockRepo, PackageCache: cache}

	err := uc.CalculateFee(context.Background(), cacheTestFeeInput(ledgerID), orgID)
	require.NoError(t, err)

	assert.Equal(t, 1, cache.getCalls, "miss must read the cache once")
	assert.Equal(t, 1, cache.setCalls, "miss must populate the cache once")
	assert.Equal(t, packageCacheNotFoundSentinel, cache.store[packageCacheKey(orgID, ledgerID)],
		"zero-package result must be stored as the NOT_FOUND sentinel")
}

// TestCalculateFee_Cache_SentinelHitSkipsMongo proves that a NOT_FOUND sentinel
// hit serves the zero-package result WITHOUT querying Mongo at all — the core
// win for the common zero-package tenant.
func TestCalculateFee_Cache_SentinelHitSkipsMongo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := pack.NewMockRepository(ctrl)
	cache := newFakePackageCache()

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Pre-seed the sentinel.
	cache.store[packageCacheKey(orgID, ledgerID)] = packageCacheNotFoundSentinel

	// Mongo MUST NOT be called — no EXPECT set; gomock fails on any call.

	uc := &UseCase{packageRepo: mockRepo, PackageCache: cache}

	err := uc.CalculateFee(context.Background(), cacheTestFeeInput(ledgerID), orgID)
	require.NoError(t, err)

	assert.Equal(t, 1, cache.getCalls, "sentinel hit reads the cache once")
	assert.Equal(t, 0, cache.setCalls, "sentinel hit must not re-write the cache")
}

// TestCalculateFee_Cache_PopulatedHitSkipsMongo proves that a populated hit
// decodes the cached package set without querying Mongo.
func TestCalculateFee_Cache_PopulatedHitSkipsMongo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := pack.NewMockRepository(ctrl)
	cache := newFakePackageCache()

	orgID := uuid.New()
	ledgerID := uuid.New()
	enable := true

	// A package whose amount window excludes the 1000 transaction value, so
	// CalculateFee finds a package (no early return) but applies no fee — keeping
	// the test focused on the cache read, not the distribution math.
	pkgs := []*pack.Package{{
		ID:            uuid.New(),
		LedgerID:      ledgerID,
		MinimumAmount: decimal.NewFromInt(5000),
		MaximumAmount: decimal.NewFromInt(9000),
		Enable:        &enable,
		Fees:          map[string]model.Fee{},
	}}
	encoded, err := json.Marshal(pkgs)
	require.NoError(t, err)

	cache.store[packageCacheKey(orgID, ledgerID)] = encoded

	// Mongo MUST NOT be called.

	uc := &UseCase{packageRepo: mockRepo, PackageCache: cache}

	err = uc.CalculateFee(context.Background(), cacheTestFeeInput(ledgerID), orgID)
	require.NoError(t, err)

	assert.Equal(t, 1, cache.getCalls, "populated hit reads the cache once")
	assert.Equal(t, 0, cache.setCalls, "populated hit must not re-write the cache")
}

// TestCalculateFee_Cache_GetErrorFallsBackToMongo proves a cache read error does
// NOT fail the request: it falls back to Mongo and re-populates the cache.
func TestCalculateFee_Cache_GetErrorFallsBackToMongo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := pack.NewMockRepository(ctrl)
	cache := newFakePackageCache()
	cache.getErr = errCacheTest

	orgID := uuid.New()
	ledgerID := uuid.New()

	mockRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{}, nil).
		Times(1)

	uc := &UseCase{packageRepo: mockRepo, PackageCache: cache}

	err := uc.CalculateFee(context.Background(), cacheTestFeeInput(ledgerID), orgID)
	require.NoError(t, err, "a cache read error must not fail the request")

	assert.Equal(t, 1, cache.getCalls)
	assert.Equal(t, 1, cache.setCalls, "fallback must still re-populate the cache")
}

// TestCalculateFee_Cache_NilCacheQueriesMongo proves a nil PackageCache disables
// caching: every call goes straight to Mongo (the pre-fix behavior).
func TestCalculateFee_Cache_NilCacheQueriesMongo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := pack.NewMockRepository(ctrl)

	orgID := uuid.New()
	ledgerID := uuid.New()

	mockRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{}, nil).
		Times(1)

	uc := &UseCase{packageRepo: mockRepo, PackageCache: nil}

	err := uc.CalculateFee(context.Background(), cacheTestFeeInput(ledgerID), orgID)
	require.NoError(t, err)
}

// TestInvalidatePackageCache_DeletesKey proves the invalidation helper removes
// the (org,ledger) key and tolerates a nil cache.
func TestInvalidatePackageCache_DeletesKey(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	key := packageCacheKey(orgID, ledgerID)

	cache := newFakePackageCache()
	cache.store[key] = packageCacheNotFoundSentinel

	uc := &UseCase{PackageCache: cache}
	uc.invalidatePackageCache(context.Background(), cacheTestLogger(), orgID, ledgerID)

	assert.Equal(t, 1, cache.delCalls, "invalidation must Del the key once")
	_, present := cache.store[key]
	assert.False(t, present, "key must be gone after invalidation")

	// Nil cache must be a safe no-op.
	ucNil := &UseCase{PackageCache: nil}
	assert.NotPanics(t, func() {
		ucNil.invalidatePackageCache(context.Background(), cacheTestLogger(), orgID, ledgerID)
	})
}

// TestPackageCacheKey_StableAndScoped locks the key format and proves distinct
// (org,ledger) pairs map to distinct keys (no cross-ledger collision).
func TestPackageCacheKey_StableAndScoped(t *testing.T) {
	t.Parallel()

	org := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerA := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	ledgerB := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	assert.Equal(t,
		"fee_packages:{11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222}",
		packageCacheKey(org, ledgerA))

	assert.NotEqual(t, packageCacheKey(org, ledgerA), packageCacheKey(org, ledgerB),
		"different ledgers under the same org must not share a cache key")
}
