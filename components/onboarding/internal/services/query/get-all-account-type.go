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

// GetAllAccountType retrieves all account types for a ledger with metadata enrichment.
//
// Account types define the classification system for accounts within a ledger.
// They determine account behavior, reporting categories, and transaction rules.
// This method fetches all account types with their associated metadata.
//
// Domain Context:
//
// Account types serve fundamental accounting purposes:
//   - ASSET: Resources owned (e.g., cash, receivables)
//   - LIABILITY: Obligations owed (e.g., payables, loans)
//   - EQUITY: Owner's interest (e.g., retained earnings)
//   - REVENUE: Income earned (e.g., sales, fees)
//   - EXPENSE: Costs incurred (e.g., salaries, utilities)
//
// Custom account types can extend this with business-specific classifications
// (e.g., "SETTLEMENT", "ESCROW", "SUSPENSE").
//
// Query Process:
//
//	Step 1: Initialize Tracing
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Fetch Account Types from PostgreSQL
//	  - Query all account types for the organization/ledger
//	  - Apply cursor-based pagination
//	  - Handle not-found with business error
//
//	Step 3: Prepare Metadata Filter
//	  - Initialize empty BSON filter if not provided
//	  - Ensures consistent MongoDB query behavior
//
//	Step 4: Fetch Metadata from MongoDB
//	  - Query metadata documents matching filter criteria
//	  - Build lookup map indexed by entity ID
//
//	Step 5: Enrich Account Types with Metadata
//	  - Assign metadata from lookup map
//	  - Convert UUID to string for map lookup
//
// Parameters:
//   - ctx: Request context with tenant and tracing information
//   - organizationID: Organization UUID for tenant isolation
//   - ledgerID: Ledger UUID to scope account types
//   - filter: Query parameters with cursor pagination and optional metadata filter
//
// Returns:
//   - []*mmodel.AccountType: Account types with metadata, nil if none found
//   - libHTTP.CursorPagination: Cursor for paginated results
//   - error: Business error (ErrNoAccountTypesFound) or infrastructure error
//
// Error Scenarios:
//   - ErrNoAccountTypesFound: No account types configured for ledger
//   - ErrEntityNotFound: Metadata lookup failed
//   - Database errors: PostgreSQL or MongoDB connection issues
//
// Pagination:
//
// This method uses cursor-based pagination for efficient traversal of large
// account type lists. The cursor encodes the position for the next page.
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
