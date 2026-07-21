// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
)

// validateInstrumentClosingDate validates the closing date of an instrument.
// It checks if the closing date is before the creation date.
func (uc *UseCase) validateInstrumentClosingDate(ctx context.Context, organizationID string, holderID, aliasId uuid.UUID, closingDate *mmodel.Date) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.validate_instrument_closing_date")
	defer span.End()

	if closingDate == nil {
		return nil
	}

	alias, err := uc.GetInstrumentByID(ctx, organizationID, holderID, aliasId, false)
	if err != nil {
		recordSpanError(span, "Failed to get alias", err)

		return err
	}

	createdAtDate := mmodel.Date{Time: alias.CreatedAt}
	if closingDate.Before(createdAtDate) {
		return pkg.ValidateBusinessError(constant.ErrInstrumentClosingDateBeforeCreation, constant.EntityInstrument)
	}

	return nil
}
