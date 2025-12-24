package services

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// validateAliasClosingDate validates the closing date of an alias
// It checks if the closing date is before the creation date
func (uc *UseCase) validateAliasClosingDate(ctx context.Context, organizationID string, holderID, aliasId uuid.UUID, closingDate *time.Time) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.validate_alias_closing_date")
	defer span.End()

	if closingDate == nil {
		return nil
	}

	alias, err := uc.GetAliasByID(ctx, organizationID, holderID, aliasId, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get alias", err)
		logger.Errorf("Failed to get alias: %v", err)

		return err
	}

	if closingDate.Before(alias.CreatedAt) {
		return pkg.ValidateBusinessError(constant.ErrAliasClosingDateBeforeCreationDate, reflect.TypeOf(mmodel.Alias{}).Name())
	}

	return nil
}
