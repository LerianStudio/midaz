package query

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

	internalKey := libCommons.SettingsTransactionInternalKey(organizationID, ledgerID, settingKey)

	cachedValue, err := uc.RedisRepo.Get(ctx, internalKey)
	if err != nil && err != redis.Nil {
		logger.Warnf("Error retrieving settings from cache: %v", err.Error())
	}

	if err == nil && cachedValue != "" {
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

	foundSetting, err := uc.SettingsRepo.FindByKey(ctx, organizationID, ledgerID, settingKey)
	if err != nil {
		if err == services.ErrDatabaseItemNotFound {
			logger.Info("Setting not found in database, creating default")

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

	err = uc.RedisRepo.Set(ctx, internalKey, strconv.FormatBool(*foundSetting.Active), 0)
	if err != nil {
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
