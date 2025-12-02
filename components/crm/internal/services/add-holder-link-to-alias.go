package services

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// AddHolderLinkToAlias adds a new holder link to an existing alias
func (uc *UseCase) AddHolderLinkToAlias(ctx context.Context, organizationID string, aliasID uuid.UUID, holderID uuid.UUID, linkType string) (*mmodel.HolderLink, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.add_holder_link_to_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.alias_id", aliasID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.link_type", linkType),
	)

	linkTypePtr := &linkType

	err := uc.ValidateLinkType(ctx, linkTypePtr)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to validate link type", err)
		logger.Errorf("Failed to validate link type: %v", err)

		return nil, err
	}

	err = uc.ValidateHolderLinkConstraints(ctx, organizationID, aliasID, linkType)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to validate holder link constraints", err)
		logger.Errorf("Failed to validate holder link constraints: %v", err)

		return nil, err
	}

	holderLinkID := libCommons.GenerateUUIDv7()
	linkTypeEnum := mmodel.LinkType(linkType)

	tpVinc, ok := mmodel.GetTpVincValue(linkTypeEnum)
	if !ok {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get TpVinc value from LinkType", nil)
		logger.Errorf("Failed to get TpVinc value for link type: %v", linkType)

		return nil, pkg.ValidateBusinessError(cn.ErrInvalidLinkType, reflect.TypeOf(mmodel.HolderLink{}).Name())
	}

	holderLink := &mmodel.HolderLink{
		ID:        &holderLinkID,
		HolderID:  &holderID,
		AliasID:   &aliasID,
		LinkType:  &linkType,
		TpVinc:    &tpVinc,
		Metadata:  make(map[string]any),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	createdHolderLink, err := uc.HolderLinkRepo.Create(ctx, organizationID, holderLink)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create holder link", err)
		logger.Errorf("Failed to create holder link: %v", err)

		return nil, err
	}

	return createdHolderLink, nil
}
