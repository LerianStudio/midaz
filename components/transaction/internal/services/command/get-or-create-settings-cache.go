package command

import (
	"context"
	"strconv"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// GetOrCreateSettingsCache retrieves a setting's active status from cache or database with fallback.
// If the setting's active status exists in cache, it returns a Settings object with the cached value.
// If not found in cache, it fetches from database and stores only the active field in cache for future use.
// The cache is persistent (no TTL) and only stores the boolean active field value.
func (uc *UseCase) GetOrCreateSettingsCache(ctx context.Context, organizationID, ledgerID uuid.UUID, settingKey string) (*mmodel.Settings, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "command.get_or_create_settings_cache")
	defer span.End()

	logger.Infof("Trying to retrieve settings cache for key: %s", settingKey)

	internalKey := libCommons.SettingsTransactionInternalKey(organizationID, ledgerID, settingKey)

	cachedValue, err := uc.RedisRepo.Get(ctx, internalKey)
	if err != nil && err != redis.Nil {
		logger.Warnf("Error retrieving settings from cache: %v", err.Error())
	}

	if err == nil && cachedValue != "" {
		logger.Infof("Settings cache hit for key: %s", settingKey)

		active, err := strconv.ParseBool(cachedValue)
		if err != nil {
			logger.Errorf("Error parsing cached active value: %v", err.Error())
		} else {
			return &mmodel.Settings{
				ID:             libCommons.GenerateUUIDv7(),
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Key:            settingKey,
				Active:         &active,
			}, nil
		}
	}

	logger.Infof("Settings default value for key: %s, fetching from database", settingKey)

	foundSetting, err := uc.SettingsRepo.FindByKey(ctx, organizationID, ledgerID, settingKey)
	if err != nil {
		if err == services.ErrDatabaseItemNotFound {
			logger.Infof("Setting not found for key: %s, creating default", settingKey)

			defaultActive := false
			foundSetting = &mmodel.Settings{
				Active: &defaultActive,
			}
		} else {
			libOpentelemetry.HandleSpanError(&span, "Failed to fetch setting from database", err)

			logger.Errorf("Error fetching setting from database: %v", err.Error())

			return nil, err
		}
	}

	if err := uc.CreateSettingsCache(ctx, organizationID, ledgerID, settingKey, foundSetting.Active); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to cache setting", err)

		logger.Warnf("Failed to cache setting with key %s: %v", settingKey, err)
	}

	if foundSetting.ID == uuid.Nil {
		return &mmodel.Settings{
			ID:             libCommons.GenerateUUIDv7(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Key:            settingKey,
			Active:         foundSetting.Active,
		}, nil
	}

	return foundSetting, nil
}
