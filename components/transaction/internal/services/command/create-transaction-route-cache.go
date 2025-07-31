package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// CreateAccountingRouteCache creates a cache for the accounting route.
// It converts the transaction route into a cache structure and stores it in Redis.
// The cache structure is a map of operation route ids to their type and account rule.
// The operation route ids are the uuids of the operation routes in the transaction route.
// The type is the type of the operation route (debit or credit).
// The account rule is the account rule of the operation route.
func (uc *UseCase) CreateAccountingRouteCache(ctx context.Context, route *mmodel.TransactionRoute) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route_cache")
	defer span.End()

	logger.Infof("Creating transaction route cache for transaction route with id: %s", route.ID)

	internalKey := libCommons.AccountingRoutesInternalKey(route.OrganizationID, route.LedgerID, route.ID)

	cacheData := route.ToCache()

	cacheBytes, err := cacheData.ToMsgpack()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert route to cache data", err)

		logger.Errorf("Failed to convert route to cache data: %v", err)

		return err
	}

	err = uc.RedisRepo.SetBytes(ctx, internalKey, cacheBytes, 0)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)

		return err
	}

	logger.Infof("Successfully created transaction route cache for transaction route with id: %s", route.ID)

	return nil
}
