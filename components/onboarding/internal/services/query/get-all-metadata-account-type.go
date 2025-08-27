package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataAccountType fetch all Account Types from the repository filtered by metadata
func (uc *UseCase) GetAllMetadataAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.AccountType, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_account_type")
	defer span.End()

	logger.Infof("Retrieving account types by metadata")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.AccountType{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAccountTypesFound, reflect.TypeOf(mmodel.AccountType{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	accountTypes, err := uc.AccountTypeRepo.ListByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		logger.Errorf("Error getting account types on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoAccountTypesFound, reflect.TypeOf(mmodel.AccountType{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account types on repo", err)

			logger.Warn("No account types found")

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account types on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	for i := range accountTypes {
		if data, ok := metadataMap[accountTypes[i].ID.String()]; ok {
			accountTypes[i].Metadata = data
		}
	}

	cur := libHTTP.CursorPagination{}

	return accountTypes, cur, nil
}
