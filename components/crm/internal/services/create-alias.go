// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) CreateAlias(ctx context.Context, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput) (*mmodel.Alias, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	aliasID := libCommons.GenerateUUIDv7()

	alias := &mmodel.Alias{
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
			libOpenTelemetry.HandleSpanError(&span, "Failed to validate related parties", err)
			logger.Errorf("Failed to validate related parties: %v", err)

			return nil, err
		}

		for _, rp := range cai.RelatedParties {
			rpID := libCommons.GenerateUUIDv7()
			rp.ID = &rpID
		}

		alias.RelatedParties = cai.RelatedParties
	}

	holder, err := uc.GetHolderByID(ctx, organizationID, holderID, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get holder by id", err)
		logger.Errorf("Failed to get holder by id %v", holderID.String())

		return nil, err
	}

	alias.Document = holder.Document
	alias.Type = holder.Type

	createdAlias, err := uc.AliasRepo.Create(ctx, organizationID, alias)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create alias", err)
		logger.Errorf("Failed to create alias: %v", err)

		return nil, err
	}

	return createdAlias, nil
}
