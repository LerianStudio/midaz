// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

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
)

// GetAllBalances retrieves a paginated list of balances with metadata.
//
// Fetches balances from PostgreSQL with cursor pagination, then enriches with MongoDB metadata.
// Returns empty array if no balances found.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - filter: Query parameters (cursor pagination, sorting, date range)
//
// Returns:
//   - []*mmodel.Balance: Array of balances with metadata
//   - libHTTP.CursorPagination: Pagination cursor info
//   - error: Business error if query fails
//
// OpenTelemetry: Creates span "query.get_all_balances"
func (uc *UseCase) GetAllBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances")
	defer span.End()

	logger.Infof("Retrieving all balances")

	balance, cur, err := uc.BalanceRepo.ListAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting balances on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoBalancesFound, reflect.TypeOf(mmodel.Balance{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances on repo", err)

			logger.Warnf("Error getting balances on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if balance != nil {
		balanceIDs := make([]string, len(balance))
		for i, b := range balance {
			balanceIDs[i] = b.ID
		}

		metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Balance{}).Name(), balanceIDs)
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation", err)

			logger.Warnf("Error getting metadata on mongodb operation: %v", err)

			return nil, libHTTP.CursorPagination{}, err
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

// GetAllBalancesByAlias retrieves all balances for an account by alias.
//
// Fetches all balance entries (default + additional) for an account identified by alias.
// Does NOT enrich with metadata (performance optimization).
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - alias: Account alias to lookup
//
// Returns:
//   - []*mmodel.Balance: Array of balances (without metadata)
//   - error: Business error if query fails
//
// OpenTelemetry: Creates span "query.get_all_balances_by_alias"
func (uc *UseCase) GetAllBalancesByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_alias")
	defer span.End()

	logger.Infof("Retrieving all balances by alias")

	balances, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, []string{alias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to list balances by alias on balance database", err)

		logger.Error("Failed to list balances by alias on balance database", err.Error())

		return nil, err
	}

	return balances, nil
}
