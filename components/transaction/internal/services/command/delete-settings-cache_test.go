package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteSettingsCacheSuccess tests successful cache deletion
func TestDeleteSettingsCacheSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test_setting_to_delete"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.DeleteSettingsCache(context.Background(), organizationID, ledgerID, settingKey)

	assert.NoError(t, err)
}

// TestDeleteSettingsCacheRedisError tests cache deletion with Redis error
func TestDeleteSettingsCacheRedisError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingKey := "test_setting_delete_error"
	redisError := errors.New("redis connection error")

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(redisError).
		Times(1)

	err := uc.DeleteSettingsCache(context.Background(), organizationID, ledgerID, settingKey)

	assert.Error(t, err)
	assert.Equal(t, redisError, err)
}
