package query

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"reflect"
)

// CountPortfolios returns the number of portfolios for the specified organization and ledger.
func (uc *UseCase) CountPortfolios(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_portfolios")
	defer span.End()

	logger.Infof("Counting portfolios for organization %s and ledger %s", organizationID, ledgerID)

	count, err := uc.PortfolioRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count portfolios on repo", err)
		logger.Errorf("Error counting portfolios on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return 0, pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		return 0, err
	}

	logger.Infof("Found %d portfolios for organization %s and ledger %s", count, organizationID, ledgerID)

	return count, nil
}
