// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
)

// CreateAccountType creates a new account type.
// It returns the created account type and an error if the operation fails.
//
// Streaming note: account_type.* events are intentionally NOT emitted —
// internal validation config; the type label is broadcast as a string
// field on account.* events.
func (uc *UseCase) CreateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountTypeInput) (_ *mmodel.AccountType, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account_type")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_account_type", start, err)
	}()

	now := time.Now()

	accountType := &mmodel.AccountType{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		Description:    payload.Description,
		KeyValue:       payload.KeyValue,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdAccountType, err := uc.AccountTypeRepo.Create(ctx, organizationID, ledgerID, accountType)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account type", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create account type", libLog.Err(err))

		return nil, err
	}

	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntityAccountType, createdAccountType.ID.String(), payload.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create metadata", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create account type metadata", libLog.Err(err))

		return nil, err
	}

	createdAccountType.Metadata = metadata

	return createdAccountType, nil
}
