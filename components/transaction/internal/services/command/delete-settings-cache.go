package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/google/uuid"
)

// DeleteSettingsCache removes a setting from the cache.
// Cache failures are logged but do not break the operation.
func (uc *UseCase) DeleteSettingsCache(ctx context.Context, organizationID, ledgerID uuid.UUID, settingKey string) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "command.delete_settings_cache")
	defer span.End()

	logger.Infof("Deleting cache for setting with key: %s", settingKey)

	internalKey := libCommons.SettingsTransactionInternalKey(organizationID, ledgerID, settingKey)

	err := uc.RedisRepo.Del(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete setting cache", err)

		logger.Warnf("Failed to invalidate cache for setting with key %s: %v", settingKey, err)

		return err
	}

	logger.Infof("Successfully invalidated cache for setting with key: %s", settingKey)

	return nil
}
