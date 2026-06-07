// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"

	// GetLedgerByID Get a ledger from the repository by given id.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetLedgerByID(ctx context.Context, organizationID, id uuid.UUID) (_ *mmodel.Ledger, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_by_id")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "get_ledger", start, err)
	}()

	ledger, err := uc.LedgerRepo.Find(ctx, organizationID, id)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting ledger on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, constant.EntityLedger)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get ledger on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, "No ledger found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get ledger on repo by id", err)

		return nil, err
	}

	if ledger != nil {
		metadata, err := uc.OnboardingMetadataRepo.FindByEntity(ctx, constant.EntityLedger, id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, constant.EntityLedger)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb ledger", err)

			logger.Log(ctx, libLog.LevelWarn, "No metadata found")

			return nil, err
		}

		if metadata != nil {
			ledger.Metadata = metadata.Data
		}
	}

	return ledger, nil
}
