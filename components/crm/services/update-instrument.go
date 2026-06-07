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
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) UpdateInstrumentByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, uai *mmodel.UpdateInstrumentInput, fieldsToRemove []string) (*mmodel.Instrument, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.update_instrument")
	defer span.End()

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

	alias := &mmodel.Instrument{
		Metadata:       uai.Metadata,
		BankingDetails: uai.BankingDetails,
		UpdatedAt:      time.Now(),
	}

	if uai.RegulatoryFields != nil {
		alias.RegulatoryFields = &mmodel.RegulatoryFields{
			ParticipantDocument: uai.RegulatoryFields.ParticipantDocument,
		}
	}

	if len(uai.RelatedParties) > 0 {
		existingAlias, err := uc.InstrumentRepo.Find(ctx, organizationID, holderID, id, false)
		if err != nil {
			recordSpanError(span, "Failed to fetch existing alias for related parties append", err)

			return nil, err
		}

		if existingAlias.RelatedParties == nil {
			alias.RelatedParties = make([]*mmodel.RelatedParty, 0, len(uai.RelatedParties))
		} else {
			alias.RelatedParties = make([]*mmodel.RelatedParty, len(existingAlias.RelatedParties), len(existingAlias.RelatedParties)+len(uai.RelatedParties))
			copy(alias.RelatedParties, existingAlias.RelatedParties)
		}

		for _, rp := range uai.RelatedParties {
			rpID, rpErr := libCommons.GenerateUUIDv7()
			if rpErr != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to generate related party id", rpErr)

				return nil, rpErr
			}

			alias.RelatedParties = append(alias.RelatedParties, &mmodel.RelatedParty{
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
			recordSpanError(span, "Failed to validate alias closing date", err)

			return nil, err
		}
	}

	updatedAlias, err := uc.InstrumentRepo.Update(ctx, organizationID, holderID, id, alias, fieldsToRemove)
	if err != nil {
		recordSpanError(span, "Failed to update alias", err)

		return nil, err
	}

	return updatedAlias, nil
}
