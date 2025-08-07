package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) GetAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_account_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	if err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", filter); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert filter to JSON string", err)
	}

	logger.Infof("Retrieving all balances by account")

	balance, cur, err := uc.BalanceRepo.ListAllByAccountID(ctx, organizationID, ledgerID, accountID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get balances on repo", err)

		logger.Errorf("Error getting balances on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoBalancesFound, reflect.TypeOf(mmodel.Balance{}).Name())
		}

		return nil, libHTTP.CursorPagination{}, err
	}

	if balance != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Balance{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range balance {
			if data, ok := metadataMap[balance[i].ID]; ok {
				balance[i].Metadata = data
			}
		}
	}

	return balance, cur, nil
}
