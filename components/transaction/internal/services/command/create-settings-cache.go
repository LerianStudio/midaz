package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/google/uuid"
)

// CreateSettingsCache creates or refreshes a settings cache entry.
// It converts the boolean active value to a string and stores it in Redis with persistent TTL.
// Cache failures are logged but do not break the operation.
func (uc *UseCase) CreateSettingsCache(ctx context.Context, organizationID, ledgerID uuid.UUID, settingKey string, active *bool) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "command.create_settings_cache")
	defer span.End()

	logger.Infof("Creating cache for setting with key: %s", settingKey)

	internalKey := libCommons.SettingsTransactionInternalKey(organizationID, ledgerID, settingKey)

	activeValue := "false"
	if active != nil && *active {
		activeValue = "true"
	}

	err := uc.RedisRepo.Set(ctx, internalKey, activeValue, 0)
	if err != nil {
		logger.Warnf("Failed to cache setting with key %s: %v", settingKey, err)

		return err
	}

	logger.Infof("Successfully cached setting with key: %s", settingKey)

	return nil
}
