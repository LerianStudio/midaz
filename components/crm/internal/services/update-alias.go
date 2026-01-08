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

	logger.Infof("Trying to update alias: %v", id.String())

	if len(uai.RelatedParties) > 0 {
		err := uc.ValidateRelatedParties(ctx, uai.RelatedParties)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to validate related parties", err)
			logger.Errorf("Failed to validate related parties: %v", err)

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
			libOpenTelemetry.HandleSpanError(&span, "Failed to fetch existing alias for related parties append", err)
			logger.Errorf("Failed to fetch existing alias: %v", err)

			return nil, err
		}

		if existingAlias.RelatedParties == nil {
			alias.RelatedParties = make([]*mmodel.RelatedParty, 0, len(uai.RelatedParties))
		} else {
			alias.RelatedParties = make([]*mmodel.RelatedParty, len(existingAlias.RelatedParties), len(existingAlias.RelatedParties)+len(uai.RelatedParties))
			copy(alias.RelatedParties, existingAlias.RelatedParties)
		}

		for _, rp := range uai.RelatedParties {
			rpID := libCommons.GenerateUUIDv7()
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
			libOpenTelemetry.HandleSpanError(&span, "Failed to validate alias closing date", err)
			logger.Errorf("Failed to validate alias closing date: %v", err)

			return nil, err
		}
	}

	updatedAlias, err := uc.AliasRepo.Update(ctx, organizationID, holderID, id, alias, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to update alias", err)
		logger.Errorf("Failed to update alias: %v", err)

		return nil, err
	}

	return updatedAlias, nil
}
