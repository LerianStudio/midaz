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

func (uc *UseCase) CreateInstrument(ctx context.Context, organizationID string, holderID uuid.UUID, cai *mmodel.CreateInstrumentInput) (_ *mmodel.Instrument, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_instrument")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "create_instrument", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	aliasID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate alias id", err)

		return nil, err
	}

	alias := &mmodel.Instrument{
		ID:        &aliasID,
		LedgerID:  &cai.LedgerID,
		AccountID: &cai.AccountID,
		HolderID:  &holderID,
		Metadata:  cai.Metadata,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if cai.BankingDetails != nil {
		alias.BankingDetails = &mmodel.BankingDetails{
			Branch:      cai.BankingDetails.Branch,
			Account:     cai.BankingDetails.Account,
			Type:        cai.BankingDetails.Type,
			OpeningDate: cai.BankingDetails.OpeningDate,
			ClosingDate: cai.BankingDetails.ClosingDate,
			IBAN:        cai.BankingDetails.IBAN,
			CountryCode: cai.BankingDetails.CountryCode,
			BankID:      cai.BankingDetails.BankID,
		}
	}

	if cai.RegulatoryFields != nil {
		alias.RegulatoryFields = &mmodel.RegulatoryFields{
			ParticipantDocument: cai.RegulatoryFields.ParticipantDocument,
		}
	}

	if len(cai.RelatedParties) > 0 {
		if err := uc.ValidateRelatedParties(ctx, cai.RelatedParties); err != nil {
			recordSpanError(span, "Failed to validate related parties", err)

			return nil, err
		}

		for _, rp := range cai.RelatedParties {
			rpID, rpErr := libCommons.GenerateUUIDv7()
			if rpErr != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to generate related party id", rpErr)

				return nil, rpErr
			}

			rp.ID = &rpID
		}

		alias.RelatedParties = cai.RelatedParties
	}

	holder, err := uc.GetHolderByID(ctx, organizationID, holderID, false)
	if err != nil {
		recordSpanError(span, "Failed to get holder by id", err)

		return nil, err
	}

	alias.Document = holder.Document
	alias.Type = holder.Type

	createdAlias, err := uc.InstrumentRepo.Create(ctx, organizationID, alias)
	if err != nil {
		recordSpanError(span, "Failed to create alias", err)

		return nil, err
	}

	return createdAlias, nil
}
