package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	redisLib "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetOrCreateSettingsCacheCacheHit tests successful cache hit returning cached active value
func TestGetOrCreateSettingsCacheCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsRepo: mockSettingsRepo,
	}

	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test-setting"

	mockRedisRepo.EXPECT().
		Get(ctx, gomock.Any()).
		Return("true", nil)

	result, err := uc.GetOrCreateSettingsCache(ctx, organizationID, ledgerID, settingKey)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, settingKey, result.Key)
	assert.NotNil(t, result.Active)
	assert.True(t, *result.Active)
}

// TestGetOrCreateSettingsCacheCacheHitWithFalse tests successful cache hit with false value
func TestGetOrCreateSettingsCacheCacheHitWithFalse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsRepo: mockSettingsRepo,
	}

	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test-setting"

	mockRedisRepo.EXPECT().
		Get(ctx, gomock.Any()).
		Return("false", nil)

	result, err := uc.GetOrCreateSettingsCache(ctx, organizationID, ledgerID, settingKey)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, settingKey, result.Key)
	assert.NotNil(t, result.Active)
	assert.False(t, *result.Active)
}

// TestGetOrCreateSettingsCacheMissFoundInDatabase tests cache miss with setting found in database
func TestGetOrCreateSettingsCacheMissFoundInDatabase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsRepo: mockSettingsRepo,
	}

	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test-setting"

	activeValue := true
	dbSetting := &mmodel.Settings{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            settingKey,
		Active:         &activeValue,
		Description:    "Test setting",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	mockRedisRepo.EXPECT().
		Get(ctx, gomock.Any()).
		Return("", redisLib.Nil)

	mockSettingsRepo.EXPECT().
		FindByKey(ctx, organizationID, ledgerID, settingKey).
		Return(dbSetting, nil)

	mockRedisRepo.EXPECT().
		Set(ctx, gomock.Any(), "true", time.Duration(0)).
		Return(nil)

	result, err := uc.GetOrCreateSettingsCache(ctx, organizationID, ledgerID, settingKey)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, dbSetting.ID, result.ID)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, settingKey, result.Key)
	assert.NotNil(t, result.Active)
	assert.True(t, *result.Active)
}

// TestGetOrCreateSettingsCacheMissNotFoundCreatesDefault tests cache miss with setting not found, creates default
func TestGetOrCreateSettingsCacheMissNotFoundCreatesDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsRepo: mockSettingsRepo,
	}

	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test-setting"

	mockRedisRepo.EXPECT().
		Get(ctx, gomock.Any()).
		Return("", redisLib.Nil)

	mockSettingsRepo.EXPECT().
		FindByKey(ctx, organizationID, ledgerID, settingKey).
		Return(nil, services.ErrDatabaseItemNotFound)

	mockRedisRepo.EXPECT().
		Set(ctx, gomock.Any(), "false", time.Duration(0)).
		Return(nil)

	result, err := uc.GetOrCreateSettingsCache(ctx, organizationID, ledgerID, settingKey)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, settingKey, result.Key)
	assert.NotNil(t, result.Active)
	assert.False(t, *result.Active)
}

// TestGetOrCreateSettingsCacheDatabaseError tests database error handling
func TestGetOrCreateSettingsCacheDatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsRepo: mockSettingsRepo,
	}

	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test-setting"

	mockRedisRepo.EXPECT().
		Get(ctx, gomock.Any()).
		Return("", redisLib.Nil)

	mockSettingsRepo.EXPECT().
		FindByKey(ctx, organizationID, ledgerID, settingKey).
		Return(nil, errors.New("database error"))

	result, err := uc.GetOrCreateSettingsCache(ctx, organizationID, ledgerID, settingKey)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database error")
}

// TestGetOrCreateSettingsCacheInvalidBooleanFallback tests cache hit with invalid boolean value falling back to database
func TestGetOrCreateSettingsCacheInvalidBooleanFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsRepo: mockSettingsRepo,
	}

	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test-setting"

	activeValue := false
	dbSetting := &mmodel.Settings{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            settingKey,
		Active:         &activeValue,
		Description:    "Test setting",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	mockRedisRepo.EXPECT().
		Get(ctx, gomock.Any()).
		Return("invalid-bool", nil)

	mockSettingsRepo.EXPECT().
		FindByKey(ctx, organizationID, ledgerID, settingKey).
		Return(dbSetting, nil)

	mockRedisRepo.EXPECT().
		Set(ctx, gomock.Any(), "false", time.Duration(0)).
		Return(nil)

	result, err := uc.GetOrCreateSettingsCache(ctx, organizationID, ledgerID, settingKey)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, dbSetting.ID, result.ID)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, settingKey, result.Key)
	assert.NotNil(t, result.Active)
	assert.False(t, *result.Active)
}

// TestGetOrCreateSettingsCacheErrorsDoNotBreakService tests that cache errors do not break service
func TestGetOrCreateSettingsCacheErrorsDoNotBreakService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsRepo: mockSettingsRepo,
	}

	ctx := context.Background()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test-setting"

	activeValue := true
	dbSetting := &mmodel.Settings{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            settingKey,
		Active:         &activeValue,
		Description:    "Test setting",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	mockRedisRepo.EXPECT().
		Get(ctx, gomock.Any()).
		Return("", errors.New("redis connection error"))

	mockSettingsRepo.EXPECT().
		FindByKey(ctx, organizationID, ledgerID, settingKey).
		Return(dbSetting, nil)

	mockRedisRepo.EXPECT().
		Set(ctx, gomock.Any(), "true", time.Duration(0)).
		Return(errors.New("redis write error"))

	result, err := uc.GetOrCreateSettingsCache(ctx, organizationID, ledgerID, settingKey)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, dbSetting.ID, result.ID)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, settingKey, result.Key)
	assert.NotNil(t, result.Active)
	assert.True(t, *result.Active)
}
