package services

import (
	"context"
	"reflect"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// ValidateLinkType validates that the linkType has a valid value if provided.
// Returns nil if linkType is empty (optional field).
func (uc *UseCase) ValidateLinkType(ctx context.Context, linkType *string) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "service.validate_link_type")
	defer span.End()

	if linkType == nil || strings.TrimSpace(*linkType) == "" {
		return nil
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.link_type", *linkType),
	)

	normalizedLinkType := strings.TrimSpace(*linkType)
	if !mmodel.IsValidLinkType(normalizedLinkType) {
		err := pkg.ValidateBusinessError(cn.ErrInvalidLinkType, reflect.TypeOf(mmodel.HolderLink{}).Name())

		libOpenTelemetry.HandleSpanError(&span, "Invalid linkType value", err)

		validTypes := mmodel.GetValidLinkTypes()
		logger.Errorf("Invalid linkType value: %s. Valid values are: %v", *linkType, validTypes)

		return err
	}

	return nil
}
