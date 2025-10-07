// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

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
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllAccountType retrieves a paginated list of account types with metadata using cursor pagination.
//
// Fetches account types from PostgreSQL with cursor-based pagination, then enriches with MongoDB metadata.
// Uses cursor pagination for better performance with large datasets. Returns empty array if no types found.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - filter: Query parameters (cursor pagination, sorting, metadata filters)
//
// Returns:
//   - []*mmodel.AccountType: Array of account types with metadata
//   - libHTTP.CursorPagination: Cursor pagination info (next cursor, has more)
//   - error: Business error if query fails
//
// Possible Errors:
//   - ErrNoAccountTypesFound: Database error occurred
//
// OpenTelemetry: Creates span "query.get_all_account_type"
func (uc *UseCase) GetAllAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.AccountType, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_account_type")
	defer span.End()

	logger.Infof("Retrieving account types")

	accountTypes, cur, err := uc.AccountTypeRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAccountTypesFound, reflect.TypeOf(mmodel.AccountType{}).Name())

			logger.Warn("No account types found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account types on repo", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account types on repo", err)

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
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb account type", err)

			return nil, libHTTP.CursorPagination{}, err
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
