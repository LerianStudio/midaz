// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) UpdateInstrumentByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, uai *mmodel.UpdateInstrumentInput, fieldsToRemove []string) (_ *mmodel.Instrument, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.update_instrument")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "update_instrument", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
	)

	if len(uai.RelatedParties) > 0 {
		err := uc.ValidateRelatedParties(ctx, uai.RelatedParties)
		if err != nil {
			recordSpanError(span, "Failed to validate related parties", err)

			return nil, err
		}
	}

	instrument := &mmodel.Instrument{
		Metadata:       uai.Metadata,
		BankingDetails: uai.BankingDetails,
		UpdatedAt:      time.Now(),
	}

	if uai.RegulatoryFields != nil {
		instrument.RegulatoryFields = &mmodel.RegulatoryFields{
			ParticipantDocument: uai.RegulatoryFields.ParticipantDocument,
		}
	}

	if len(uai.RelatedParties) > 0 {
		existingInstrument, err := uc.InstrumentRepo.Find(ctx, organizationID, holderID, id, false)
		if err != nil {
			recordSpanError(span, "Failed to fetch existing instrument for related parties append", err)

			return nil, err
		}

		if existingInstrument.RelatedParties == nil {
			instrument.RelatedParties = make([]*mmodel.RelatedParty, 0, len(uai.RelatedParties))
		} else {
			instrument.RelatedParties = make([]*mmodel.RelatedParty, len(existingInstrument.RelatedParties), len(existingInstrument.RelatedParties)+len(uai.RelatedParties))
			copy(instrument.RelatedParties, existingInstrument.RelatedParties)
		}

		for _, rp := range uai.RelatedParties {
			rpID, rpErr := libCommons.GenerateUUIDv7()
			if rpErr != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to generate related party id", rpErr)

				return nil, rpErr
			}

			instrument.RelatedParties = append(instrument.RelatedParties, &mmodel.RelatedParty{
				ID:        &rpID,
				Document:  rp.Document,
				Name:      rp.Name,
				Role:      rp.Role,
				StartDate: rp.StartDate,
				EndDate:   rp.EndDate,
			})
		}
	}

	if uai.BankingDetails != nil && uai.BankingDetails.ClosingDate != nil {
		err := uc.validateInstrumentClosingDate(ctx, organizationID, holderID, id, uai.BankingDetails.ClosingDate)
		if err != nil {
			recordSpanError(span, "Failed to validate instrument closing date", err)

			return nil, err
		}
	}

	updatedInstrument, err := uc.InstrumentRepo.Update(ctx, organizationID, holderID, id, instrument, fieldsToRemove)
	if err != nil {
		recordSpanError(span, "Failed to update instrument", err)

		return nil, err
	}

	return updatedInstrument, nil
}
