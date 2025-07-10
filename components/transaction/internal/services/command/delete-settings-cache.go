package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/google/uuid"
)

// DeleteSettingsCache removes a setting from the cache.
func (uc *UseCase) DeleteSettingsCache(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "command.delete_settings_cache")
	defer span.End()

	setting, err := uc.SettingsRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find setting by id", err)

		logger.Errorf("Failed to find setting by id: %v", err)

		return err
	}

	logger.Infof("Deleting cache for setting with key: %s", setting.Key)

	internalKey := libCommons.SettingsTransactionInternalKey(organizationID, ledgerID, setting.Key)

	err = uc.RedisRepo.Del(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete setting cache", err)

		logger.Warnf("Failed to invalidate cache for setting with key %s: %v", setting.Key, err)

		return err
	}

	logger.Infof("Successfully invalidated cache for setting with key: %s", setting.Key)

	return nil
}
