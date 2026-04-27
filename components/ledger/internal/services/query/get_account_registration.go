// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAccountRegistration returns the durable saga record identified by (organizationID,
// ledgerID, id). It is a thin wrapper over the repository's FindByID so the handler
// layer has a CQRS-clean entry point. Missing records yield the business error
// ErrAccountRegistrationNotFound (404) via the repository.
func (uc *UseCase) GetAccountRegistration(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountRegistration, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_registration")
	defer span.End()

	reg, err := uc.AccountRegistrationRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to fetch account registration", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to fetch account registration",
			libLog.String("organization_id", organizationID.String()),
			libLog.String("ledger_id", ledgerID.String()),
			libLog.String("id", id.String()),
			libLog.Err(err))

		return nil, err
	}

	return reg, nil
}
