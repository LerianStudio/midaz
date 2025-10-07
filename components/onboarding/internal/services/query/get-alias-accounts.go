// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// ListAccountsByAlias retrieves multiple accounts by their aliases.
//
// Batch retrieves accounts from PostgreSQL by their aliases. Does NOT enrich with metadata (performance optimization).
// Used for transaction processing where multiple accounts need to be fetched by alias.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - aliases: Array of account aliases to retrieve
//
// Returns:
//   - []*mmodel.Account: Array of found accounts (without metadata)
//   - error: Business error if query fails
//
// Possible Errors:
//   - ErrFailedToRetrieveAccountsByAliases: None of the aliases were found or query failed
//
// OpenTelemetry: Creates span "query.ListAccountsByAlias"
func (uc *UseCase) ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.ListAccountsByAlias")
	defer span.End()

	logger.Infof("Retrieving account for alias: %s", aliases)

	accounts, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		logger.Errorf("Error getting accounts on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("No accounts found for alias: %s", aliases)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Accounts by aliases", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Accounts by aliases", err)

		return nil, err
	}

	return accounts, nil
}
