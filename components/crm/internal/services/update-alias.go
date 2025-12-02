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

	if uai.AddHolderLink != nil {
		linkHolderID, err := uuid.Parse(uai.AddHolderLink.HolderID)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to parse holder ID for new link", err)
			logger.Errorf("Failed to parse holder ID for new link: %v", err)

			return nil, err
		}

		_, err = uc.AddHolderLinkToAlias(ctx, organizationID, id, linkHolderID, uai.AddHolderLink.LinkType)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to add holder link to alias", err)
			logger.Errorf("Failed to add holder link to alias: %v", err)

			return nil, err
		}
	}

	alias := &mmodel.Alias{
		Metadata:       uai.Metadata,
		BankingDetails: uai.BankingDetails,
		UpdatedAt:      time.Now(),
	}

	updatedAlias, err := uc.AliasRepo.Update(ctx, organizationID, holderID, id, alias, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to update alias", err)

		logger.Errorf("Failed to update alias: %v", err)

		return nil, err
	}

	err = uc.enrichAliasWithLinkType(ctx, organizationID, updatedAlias)
	if err != nil {
		logger.Warnf("Failed to enrich alias with link type: %v", err)
	}

	return updatedAlias, nil
}
