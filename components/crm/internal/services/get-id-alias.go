package services

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetAliasByID retrieves alias by id and its holder id
func (uc *UseCase) GetAliasByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Alias, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_alias_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	)

	logger.Infof("Get alias by id %v from holder %v", id, holderID)

	alias, err := uc.AliasRepo.Find(ctx, organizationID, holderID, id, includeDeleted)
	if err != nil {
		logger.Errorf("Failed to get alias by id %v", id)

		if errors.Is(err, cn.ErrAliasNotFound) {
			err := pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())

			libOpenTelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get alias by id", err)

			return nil, err
		}

		libOpenTelemetry.HandleSpanError(&span, "Failed to get alias by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Alias{}).Name())
	}

	uc.enrichAliasWithLinkType(ctx, organizationID, alias)

	return alias, nil
}
