package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateSettingsCacheSuccess tests successful cache creation with active=true
func TestCreateSettingsCacheSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test_setting_key"
	active := true

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), "true", time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateSettingsCache(context.Background(), organizationID, ledgerID, settingKey, &active)

	assert.NoError(t, err)
}

// TestCreateSettingsCacheSuccessWithFalse tests successful cache creation with active=false
func TestCreateSettingsCacheSuccessWithFalse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test_setting_key_false"
	active := false

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), "false", time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateSettingsCache(context.Background(), organizationID, ledgerID, settingKey, &active)

	assert.NoError(t, err)
}

// TestCreateSettingsCacheWithNilActive tests cache creation with nil active (defaults to false)
func TestCreateSettingsCacheWithNilActive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test_setting_key_nil"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), "false", time.Duration(0)).
		Return(nil).
		Times(1)

	err := uc.CreateSettingsCache(context.Background(), organizationID, ledgerID, settingKey, nil)

	assert.NoError(t, err)
}

// TestCreateSettingsCacheRedisError tests cache creation with Redis error
func TestCreateSettingsCacheRedisError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test_setting_key_error"
	active := true
	redisError := errors.New("redis connection error")

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), "true", time.Duration(0)).
		Return(redisError).
		Times(1)

	err := uc.CreateSettingsCache(context.Background(), organizationID, ledgerID, settingKey, &active)

	assert.Error(t, err)
	assert.Equal(t, redisError, err)
}
