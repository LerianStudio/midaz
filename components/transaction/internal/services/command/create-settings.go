package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateSettings creates a new setting.
// It returns the created setting and an error if the operation fails.
func (uc *UseCase) CreateSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateSettingsInput) (*mmodel.Settings, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_settings")
	defer span.End()

	now := time.Now()

	settings := &mmodel.Settings{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            payload.Key,
		Active:         &payload.Active,
		Description:    payload.Description,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdSettings, err := uc.SettingsRepo.Create(ctx, organizationID, ledgerID, settings)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create settings", err)

		logger.Errorf("Failed to create settings: %v", err)

		return nil, err
	}

	logger.Infof("Successfully created setting with key: %s", createdSettings.Key)

	if err := uc.CreateSettingsCache(ctx, organizationID, ledgerID, createdSettings.Key, createdSettings.Active); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to cache created setting", err)

		logger.Warnf("Failed to cache created setting with key %s: %v", createdSettings.Key, err)
	}

	return createdSettings, nil
}
