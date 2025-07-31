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
	"github.com/google/uuid"
	"reflect"
)

// CountLedgers returns the total count of ledgers for a specific organization
func (uc *UseCase) CountLedgers(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_ledgers")
	defer span.End()

	logger.Infof("Counting ledgers for organization: %s", organizationID)

	count, err := uc.LedgerRepo.Count(ctx, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count ledgers on repo", err)
		logger.Errorf("Error counting ledgers on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return 0, pkg.ValidateBusinessError(constant.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		return 0, err
	}

	return count, nil
}
