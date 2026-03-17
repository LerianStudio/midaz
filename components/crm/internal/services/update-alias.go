// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) UpdateAliasByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, uai *mmodel.UpdateAliasInput, fieldsToRemove []string) (*mmodel.Alias, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.update_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to update alias: %v", id.String()))

	if len(uai.RelatedParties) > 0 {
		err := uc.ValidateRelatedParties(ctx, uai.RelatedParties)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to validate related parties", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate related parties: %v", err))

			return nil, err
		}
	}

	alias := &mmodel.Alias{
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
		existingAlias, err := uc.AliasRepo.Find(ctx, organizationID, holderID, id, false)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to fetch existing alias for related parties append", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to fetch existing alias: %v", err))

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
				libOpenTelemetry.HandleSpanError(span, "Failed to generate related party id", rpErr)
				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to generate related party id: %v", rpErr))

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
		err := uc.validateAliasClosingDate(ctx, organizationID, holderID, id, uai.BankingDetails.ClosingDate)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to validate alias closing date", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate alias closing date: %v", err))

			return nil, err
		}
	}

	updatedAlias, err := uc.AliasRepo.Update(ctx, organizationID, holderID, id, alias, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to update alias", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update alias: %v", err))

		return nil, err
	}

	return updatedAlias, nil
}
