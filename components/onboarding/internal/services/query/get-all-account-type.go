package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllAccountType fetch all Account Types from the repository
func (uc *UseCase) GetAllAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.AccountType, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_account_type")
	defer span.End()

	logger.Infof("Retrieving account types")

	accountTypes, cur, err := uc.AccountTypeRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get account types on repo", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoAccountTypesFound, reflect.TypeOf(mmodel.AccountType{}).Name())
		}

		logger.Errorf("Error getting account types on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if accountTypes != nil {
		metadataFilter := filter
		if metadataFilter.Metadata == nil {
			metadataFilter.Metadata = &bson.M{}
		}

		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.AccountType{}).Name(), metadataFilter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb account type", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range accountTypes {
			if data, ok := metadataMap[accountTypes[i].ID.String()]; ok {
				accountTypes[i].Metadata = data
			}
		}
	}

	return accountTypes, cur, nil
}
