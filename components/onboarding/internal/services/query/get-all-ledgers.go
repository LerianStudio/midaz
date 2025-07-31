package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"reflect"
)

// GetAllLedgers fetch all Ledgers from the repository
func (uc *UseCase) GetAllLedgers(ctx context.Context, organizationID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Ledger, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_ledgers")
	defer span.End()

	logger.Infof("Retrieving ledgers")

	ledgers, err := uc.LedgerRepo.FindAll(ctx, organizationID, filter.ToOffsetPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get ledgers on repo", err)

		logger.Errorf("Error getting ledgers on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		return nil, err
	}

	if ledgers != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

			return nil, pkg.ValidateBusinessError(constant.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range ledgers {
			if data, ok := metadataMap[ledgers[i].ID]; ok {
				ledgers[i].Metadata = data
			}
		}
	}

	return ledgers, nil
}
