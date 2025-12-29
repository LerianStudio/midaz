package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAccountByID get an Account from the repository by given id.
func (uc *UseCase) GetAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	// portfolioID is optional (can be nil pointer)
	assert.That(id != uuid.Nil, "accountID must not be nil UUID",
		"accountID", id)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_by_id")
	defer span.End()

	logger.Infof("Retrieving account for id: %s", id)

	account, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Errorf("Error getting account on repo by id: %v", err)

		logger.Errorf("Error getting account on repo by id: %v", err)

		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	if account != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String())
		if err != nil {
			logger.Errorf("Error get metadata on mongodb account: %v", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
		}

		if metadata != nil {
			account.Metadata = metadata.Data
		}
	}

	return account, nil
}
