// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"

	libObs "github.com/LerianStudio/lib-observability"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// validateInstrumentClosingDate validates the closing date of an instrument
	// It checks if the closing date is before the creation date
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) validateInstrumentClosingDate(ctx context.Context, organizationID string, holderID, aliasId uuid.UUID, closingDate *mmodel.Date) error {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.validate_instrument_closing_date")
	defer span.End()

	if closingDate == nil {
		return nil
	}

	alias, err := uc.GetInstrumentByID(ctx, organizationID, holderID, aliasId, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get alias", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get alias: %v", err))

		return err
	}

	createdAtDate := mmodel.Date{Time: alias.CreatedAt}
	if closingDate.Before(createdAtDate) {
		return pkg.ValidateBusinessError(constant.ErrInstrumentClosingDateBeforeCreation, constant.EntityInstrument)
	}

	return nil
}
