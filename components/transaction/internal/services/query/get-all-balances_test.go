package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllBalances(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{
		Limit:        10,
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	t.Parallel()
	t.Run("SuccessNoCacheOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias",
			Key:       "k1",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(100),
			OnHold:    decimal.NewFromInt(10),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(100)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(10)))
		assert.Equal(t, int64(0), res[0].Version)
	})

	t.Run("SuccessWithCacheOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias2",
			Key:       "k2",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(1),
			OnHold:    decimal.NewFromInt(2),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
		cached := mmodel.BalanceRedis{
			Available: decimal.NewFromInt(999),
			OnHold:    decimal.NewFromInt(777),
			Version:   5,
		}
		data, _ := json.Marshal(cached)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{key: string(data)}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(999)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(777)))
		assert.Equal(t, int64(5), res[0].Version)
	})

	t.Run("RedisErrorShouldNotFail", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias3",
			Key:       "k3",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(5),
			OnHold:    decimal.NewFromInt(6),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
		_ = key // key construction is deterministic; expectation uses Any to avoid tight coupling
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("redis down")).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(5)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(6)))
		assert.Equal(t, int64(0), res[0].Version)
	})

	t.Run("InvalidCachePayloadSkipsOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias4",
			Key:       "k4",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(50),
			OnHold:    decimal.NewFromInt(7),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{key: "not-json"}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		// Should keep DB values because cache unmarshalling fails
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(50)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(7)))
		assert.Equal(t, int64(0), res[0].Version)
	})

	t.Run("CacheDecimalValuesAsString", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias5",
			Key:       "k5",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(0),
			OnHold:    decimal.NewFromInt(0),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
		cachedJSON := `{"available":"123.4500","onHold":"0.5500","version":12}`

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{key: cachedJSON}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		wantAvail, _ := decimal.NewFromString("123.4500")
		wantHold, _ := decimal.NewFromString("0.5500")
		assert.True(t, res[0].Available.Equal(wantAvail))
		assert.True(t, res[0].OnHold.Equal(wantHold))
		assert.Equal(t, int64(12), res[0].Version)
	})

	t.Run("IgnoreUnrelatedCacheKeys", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias7",
			Key:       "k7",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(10),
			OnHold:    decimal.NewFromInt(1),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
		requested := mmodel.BalanceRedis{Available: decimal.NewFromInt(777), OnHold: decimal.NewFromInt(333), Version: 42}
		reqData, _ := json.Marshal(requested)
		unrelated := mmodel.BalanceRedis{Available: decimal.NewFromInt(9999), OnHold: decimal.NewFromInt(9999), Version: 999}
		unrelData, _ := json.Marshal(unrelated)

		redisMap := map[string]string{
			key:                 string(reqData),
			"unrelated:somekey": string(unrelData),
		}

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(redisMap, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(777)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(333)))
		assert.Equal(t, int64(42), res[0].Version)
	})

	t.Run("MixedValidAndInvalidCacheEntries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		b1 := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias8",
			Key:       "k8",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(10),
			OnHold:    decimal.NewFromInt(1),
		}
		b2 := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias9",
			Key:       "k9",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(20),
			OnHold:    decimal.NewFromInt(3),
		}
		balances := []*mmodel.Balance{b1, b2}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		k1 := utils.BalanceInternalKey(organizationID, ledgerID, b1.Alias+"#"+b1.Key)
		k2 := utils.BalanceInternalKey(organizationID, ledgerID, b2.Alias+"#"+b2.Key)

		valid := mmodel.BalanceRedis{Available: decimal.NewFromInt(111), OnHold: decimal.NewFromInt(9), Version: 7}
		validJSON, _ := json.Marshal(valid)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{k1, k2}).
			Return(map[string]string{k1: string(validJSON), k2: "not-json"}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 2)
		assert.Equal(t, mockCur, cur)
		// b1 overlaid
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(111)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(9)))
		assert.Equal(t, int64(7), res[0].Version)
		// b2 unchanged due to invalid cache value
		assert.True(t, res[1].Available.Equal(decimal.NewFromInt(20)))
		assert.True(t, res[1].OnHold.Equal(decimal.NewFromInt(3)))
		assert.Equal(t, int64(0), res[1].Version)
	})

	// Same alias with two keys; overlay should only apply to the matching alias#key
	t.Run("OverlayOnlyForMatchingAliasAndKeySameAliasMultiKeys", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		alias := "@alias_same"
		bDefault := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     alias,
			Key:       "default",
			AssetCode: "BRL",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
		bExtra := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     alias,
			Key:       "key-1",
			AssetCode: "BRL",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
		balances := []*mmodel.Balance{bDefault, bExtra}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		kDefault := utils.BalanceInternalKey(organizationID, ledgerID, bDefault.Alias+"#"+bDefault.Key)
		kExtra := utils.BalanceInternalKey(organizationID, ledgerID, bExtra.Alias+"#"+bExtra.Key)
		overlay := mmodel.BalanceRedis{Available: decimal.NewFromInt(42), OnHold: decimal.Zero, Version: 99}
		overlayJSON, _ := json.Marshal(overlay)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{kDefault, kExtra}).
			Return(map[string]string{kDefault: string(overlayJSON)}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 2)
		assert.Equal(t, mockCur, cur)
		// default overlaid
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(42)))
		assert.True(t, res[0].OnHold.Equal(decimal.Zero))
		assert.Equal(t, int64(99), res[0].Version)
		// key-1 unchanged (no cache entry)
		assert.True(t, res[1].Available.Equal(decimal.Zero))
		assert.True(t, res[1].OnHold.Equal(decimal.Zero))
		assert.Equal(t, int64(0), res[1].Version)
	})

	// Partial overlay where one cache entry is missing entirely
	t.Run("PartialOverlayMissingCacheEntry", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bA := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@aliasA",
			Key:       "default",
			AssetCode: "BRL",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
		bB := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@aliasB",
			Key:       "default",
			AssetCode: "BRL",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
		balances := []*mmodel.Balance{bA, bB}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		kA := utils.BalanceInternalKey(organizationID, ledgerID, bA.Alias+"#"+bA.Key)
		kB := utils.BalanceInternalKey(organizationID, ledgerID, bB.Alias+"#"+bB.Key)
		overlayA := mmodel.BalanceRedis{Available: decimal.NewFromInt(77), OnHold: decimal.Zero, Version: 71}
		overlayAJSON, _ := json.Marshal(overlayA)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{kA, kB}).
			Return(map[string]string{kA: string(overlayAJSON)}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 2)
		assert.Equal(t, mockCur, cur)
		// aliasA overlaid, aliasB unchanged
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(77)))
		assert.True(t, res[0].OnHold.Equal(decimal.Zero))
		assert.Equal(t, int64(71), res[0].Version)
		assert.True(t, res[1].Available.Equal(decimal.Zero))
		assert.True(t, res[1].OnHold.Equal(decimal.Zero))
		assert.Equal(t, int64(0), res[1].Version)
	})

	t.Run("VeryLargeDecimalMagnitudesStrings", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias_big",
			Key:       "kBig",
			AssetCode: "BRL",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
		largeAvail := "123456789012345678901234567890.123456789012345678901234567890"
		largeHold := "987654321098765432109876543210.987654321098765432109876543210"
		cachedJSON := fmt.Sprintf(`{"available":"%s","onHold":"%s","version":77}`, largeAvail, largeHold)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{key: cachedJSON}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		wantAvail, _ := decimal.NewFromString(largeAvail)
		wantHold, _ := decimal.NewFromString(largeHold)
		assert.True(t, res[0].Available.Equal(wantAvail))
		assert.True(t, res[0].OnHold.Equal(wantHold))
		assert.Equal(t, int64(77), res[0].Version)
	})

	t.Run("ContextCancellationPropagatesAndSkipsOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		// Canceled context before call
		cctx, cancel := context.WithCancel(context.Background())
		cancel()

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, context.Canceled).
			Times(1)

		res, cur, err := uc.GetAllBalances(cctx, organizationID, ledgerID, filter)

		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		// If overlay (MGet) were attempted, gomock would fail due to unexpected call
	})

	t.Run("CacheDecimalValuesAsNumbers", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		bal := &mmodel.Balance{
			ID:        uuid.New().String(),
			Alias:     "@alias6",
			Key:       "k6",
			AssetCode: "BRL",
			Available: decimal.NewFromInt(0),
			OnHold:    decimal.NewFromInt(0),
		}
		balances := []*mmodel.Balance{bal}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, bal.Alias+"#"+bal.Key)
		cachedJSON := `{"available":123.45,"onHold":0.55,"version":2}`

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{key: cachedJSON}, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, mockCur, cur)
		wantAvail, _ := decimal.NewFromString("123.45")
		wantHold, _ := decimal.NewFromString("0.55")
		assert.True(t, res[0].Available.Equal(wantAvail))
		assert.True(t, res[0].OnHold.Equal(wantHold))
		assert.Equal(t, int64(2), res[0].Version)
	})

	t.Run("LargeBatchMGetOverlayAll", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		// Prepare a large list of balances
		N := 300
		balances := make([]*mmodel.Balance, 0, N)
		keys := make([]string, 0, N)
		redisMap := make(map[string]string, N)

		for i := 0; i < N; i++ {
			b := &mmodel.Balance{
				ID:        uuid.New().String(),
				Alias:     fmt.Sprintf("@a%d", i),
				Key:       "default",
				AssetCode: "BRL",
				Available: decimal.Zero,
				OnHold:    decimal.Zero,
			}
			balances = append(balances, b)
			k := utils.BalanceInternalKey(organizationID, ledgerID, b.Alias+"#"+b.Key)
			keys = append(keys, k)
			cached := mmodel.BalanceRedis{Available: decimal.NewFromInt(int64(i)), OnHold: decimal.NewFromInt(int64(i % 7)), Version: int64(i)}
			data, _ := json.Marshal(cached)
			redisMap[k] = string(data)
		}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), keys).
			Return(redisMap, nil).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, N)
		assert.Equal(t, mockCur, cur)

		// Spot-check a few positions
		check := func(idx int) {
			wantAvail := decimal.NewFromInt(int64(idx))
			wantHold := decimal.NewFromInt(int64(idx % 7))
			assert.True(t, res[idx].Available.Equal(wantAvail))
			assert.True(t, res[idx].OnHold.Equal(wantHold))
			assert.Equal(t, int64(idx), res[idx].Version)
		}

		check(0)
		check(N / 2)
		check(N - 1)
	})

	t.Run("RepoError", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		expectedErr := errors.New("errDatabaseItemNotFound")
		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, expectedErr).
			Times(1)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.Error(t, err)
		// Check that the error chain contains our expected error
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	t.Run("NoBalancesFound", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		mockBalanceRepo.
			EXPECT().
			ListAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{}, mockCur, nil).
			Times(1)

		// Ensure no Redis overlay is attempted when no balances are found
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Any()).
			Times(0)

		res, cur, err := uc.GetAllBalances(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})
}

func TestGetAllBalancesByAlias(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	alias := "test-alias"

	t.Parallel()

	t.Run("SuccessNoCacheOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		balances := []*mmodel.Balance{
			{
				ID:        "account-id-1",
				AccountID: "account-id-1",
				Alias:     alias,
				Key:       "default",
				AssetCode: "BRL",
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(0),
			},
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(balances, nil).
			Times(1)

		// Expect an MGet call with proper key and return nothing to overlay
		key := utils.BalanceInternalKey(organizationID, ledgerID, alias+"#default")
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{}, nil).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
		}

		expectedErr := errors.New("error getting balances")

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(nil, expectedErr).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.Error(t, err)
		// Check that the error chain contains our expected error
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, res)
	})

	t.Run("SuccessWithCacheOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		balances := []*mmodel.Balance{
			{
				ID:        "account-id-2",
				AccountID: "account-id-2",
				Alias:     alias,
				Key:       "default",
				AssetCode: "BRL",
				Available: decimal.NewFromInt(0),
				OnHold:    decimal.NewFromInt(0),
			},
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(balances, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, alias+"#default")
		cached := mmodel.BalanceRedis{Available: decimal.NewFromInt(999), OnHold: decimal.NewFromInt(777), Version: 12}
		data, _ := json.Marshal(cached)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{key: string(data)}, nil).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(999)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(777)))
		assert.Equal(t, int64(12), res[0].Version)
	})

	t.Run("RedisErrorShouldNotFail", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		balances := []*mmodel.Balance{
			{
				ID:        "account-id-3",
				AccountID: "account-id-3",
				Alias:     alias,
				Key:       "default",
				AssetCode: "BRL",
				Available: decimal.NewFromInt(10),
				OnHold:    decimal.NewFromInt(1),
			},
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(balances, nil).
			Times(1)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("redis down")).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(10)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(1)))
		assert.Equal(t, int64(0), res[0].Version)
	})

	t.Run("InvalidCachePayloadSkipsOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		balances := []*mmodel.Balance{
			{
				ID:        "account-id-4",
				AccountID: "account-id-4",
				Alias:     alias,
				Key:       "default",
				AssetCode: "BRL",
				Available: decimal.NewFromInt(50),
				OnHold:    decimal.NewFromInt(5),
			},
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(balances, nil).
			Times(1)

		key := utils.BalanceInternalKey(organizationID, ledgerID, alias+"#default")
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{key}).
			Return(map[string]string{key: "not-json"}, nil).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(50)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(5)))
		assert.Equal(t, int64(0), res[0].Version)
	})

	t.Run("NoBalancesFoundForAlias", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return([]*mmodel.Balance{}, nil).
			Times(1)

		// Ensure no Redis overlay is attempted when no balances are found
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Any()).
			Times(0)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.NoError(t, err)
		assert.Nil(t, res)
	})

	t.Run("MultipleKeysPartialOverlay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
			RedisRepo:   mockRedisRepo,
		}

		b1 := &mmodel.Balance{ID: "1", AccountID: "1", Alias: alias, Key: "default", AssetCode: "BRL", Available: decimal.Zero, OnHold: decimal.Zero}
		b2 := &mmodel.Balance{ID: "2", AccountID: "2", Alias: alias, Key: "k1", AssetCode: "BRL", Available: decimal.Zero, OnHold: decimal.Zero}
		balances := []*mmodel.Balance{b1, b2}

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(balances, nil).
			Times(1)

		k1 := utils.BalanceInternalKey(organizationID, ledgerID, alias+"#default")
		// No entry for k2 to simulate partial overlay
		cached := mmodel.BalanceRedis{Available: decimal.NewFromInt(42), OnHold: decimal.Zero, Version: 9}
		data, _ := json.Marshal(cached)

		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), []string{k1, utils.BalanceInternalKey(organizationID, ledgerID, alias+"#k1")}).
			Return(map[string]string{k1: string(data)}, nil).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.NoError(t, err)
		assert.Len(t, res, 2)
		assert.True(t, res[0].Available.Equal(decimal.NewFromInt(42)))
		assert.True(t, res[0].OnHold.Equal(decimal.Zero))
		assert.Equal(t, int64(9), res[0].Version)
		assert.True(t, res[1].Available.Equal(decimal.Zero))
		assert.True(t, res[1].OnHold.Equal(decimal.Zero))
		assert.Equal(t, int64(0), res[1].Version)
	})
}
