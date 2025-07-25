package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// CreateSettings creates new settings and persists data in the repository.
func (uc *UseCase) CreateSettings(ctx context.Context, organizationID, ledgerID string, settings *mmodel.CreateSettingsInput) (*mmodel.Settings, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_settings")
	defer span.End()

	logger.Infof("Trying to create settings for org: %s, ledger: %s, app: %s", organizationID, ledgerID, settings.ApplicationName)

	newSettings := &mmodel.Settings{
		ID:              primitive.NewObjectID(),
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		ApplicationName: settings.ApplicationName,
		Settings:        settings.Settings,
		Enabled:         settings.Enabled,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "settings_repository_input", newSettings)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert settings repository input to JSON string", err)

		return nil, err
	}

	err = uc.SettingsRepo.Create(ctx, newSettings)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create settings on repository", err)

		logger.Errorf("Error creating settings: %v", err)

		return nil, err
	}

	return newSettings, nil
}
