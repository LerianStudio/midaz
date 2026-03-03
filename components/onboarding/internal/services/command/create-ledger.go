// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateLedger creates a new ledger persists data in the repository.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_ledger")
	defer span.End()

	logger.Infof("Trying to create ledger: %v", cli)

	var status mmodel.Status
	if cli.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cli.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cli.Status
	}

	status.Description = cli.Status.Description

	_, err := uc.LedgerRepo.FindByName(ctx, organizationID, cli.Name)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find ledger by name", err)

		logger.Errorf("Error creating ledger: %v", err)

		return nil, err
	}

	// Validate settings early when provided, same as UpdateLedgerSettings (fail before creating the ledger).
	var settingsToPersist map[string]any

	if !mmodel.LedgerSettingsIsDefault(cli.Settings) {
		settingsMap := mmodel.LedgerSettingsToMap(*cli.Settings)

		if err := mmodel.ValidateSettings(settingsMap); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Settings validation failed", err)

			logger.Errorf("Settings validation failed: %v", err)

			return nil, err
		}

		settingsToPersist = mmodel.MergeSettingsWithDefaults(settingsMap)
	}

	ledger := &mmodel.Ledger{
		OrganizationID: organizationID.String(),
		Name:           cli.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Settings:       settingsToPersist,
	}

	led, err := uc.LedgerRepo.Create(ctx, ledger)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger", err)

		logger.Errorf("Error creating ledger: %v", err)

		return nil, err
	}

	takeName := reflect.TypeOf(mmodel.Ledger{}).Name()

	metadata, err := uc.CreateMetadata(ctx, takeName, led.ID, cli.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger metadata", err)

		logger.Errorf("Error creating ledger metadata: %v", err)

		return nil, err
	}

	led.Metadata = metadata

	// Invalidate settings cache when we persisted settings so GetLedgerSettings sees fresh data.
	if settingsToPersist != nil {
		if ledgerID, parseErr := uuid.Parse(led.ID); parseErr == nil {
			uc.invalidateSettingsCache(ctx, organizationID, ledgerID)
		}
	}

	return led, nil
}
