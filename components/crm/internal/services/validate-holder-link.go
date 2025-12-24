package services

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// ValidateHolderLinkConstraints validates business rules for holder link creation/update
func (uc *UseCase) ValidateHolderLinkConstraints(ctx context.Context, organizationID string, aliasID uuid.UUID, linkType string) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.validate_holder_link_constraints")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.alias_id", aliasID.String()),
		attribute.String("app.request.link_type", linkType),
	)

	// Rule: Only one PRIMARY_HOLDER per alias
	// Rule: Prevent duplicate holder links with same alias_id and link_type combination
	// This is already enforced by the unique index, but we validate it here for better error messages
	existingLink, err := uc.HolderLinkRepo.FindByAliasIDAndLinkType(ctx, organizationID, aliasID, linkType, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to check for existing holder link", err)
		logger.Errorf("Failed to check for existing holder link: %v", err)

		return pkg.ValidateInternalError(err, "CRM")
	}

	if existingLink != nil {
		linkTypeEnum := mmodel.LinkType(linkType)
		if linkTypeEnum == mmodel.LinkTypePrimaryHolder {
			libOpenTelemetry.HandleSpanError(&span, "Primary holder already exists for this alias", constant.ErrPrimaryHolderAlreadyExists)

			logger.Errorf("Primary holder already exists for alias %v", aliasID.String())

			return pkg.ValidateBusinessError(constant.ErrPrimaryHolderAlreadyExists, reflect.TypeOf(mmodel.HolderLink{}).Name())
		}

		libOpenTelemetry.HandleSpanError(&span, "Holder link already exists with same alias and link type", constant.ErrDuplicateHolderLink)

		logger.Errorf("Holder link already exists for alias %v with link type %v", aliasID.String(), linkType)

		return pkg.ValidateBusinessError(constant.ErrDuplicateHolderLink, reflect.TypeOf(mmodel.HolderLink{}).Name())
	}

	return nil
}
