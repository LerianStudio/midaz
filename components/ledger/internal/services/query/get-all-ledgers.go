package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/google/uuid"
)

// GetAllLedgers fetch all Ledgers from the repository
func (uc *UseCase) GetAllLedgers(ctx context.Context, organizationID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Ledger, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_ledgers")
	defer span.End()

	logger.Infof("Retrieving ledgers")

	ledgers, err := uc.LedgerRepo.FindAll(ctx, organizationID, filter.ToOffsetPagination())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get ledgers on repo", err)

		logger.Errorf("Error getting ledgers on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		return nil, err
	}

	if ledgers != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

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
