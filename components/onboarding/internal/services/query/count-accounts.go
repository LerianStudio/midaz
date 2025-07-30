package query

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"reflect"
)

// CountAccounts returns the number of accounts for the specified organization, ledger and optional portfolio.
func (uc *UseCase) CountAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_accounts")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	count, err := uc.AccountRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count accounts on repo", err)
		logger.Errorf("Error counting accounts on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return 0, pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return 0, err
	}

	return count, nil
}
