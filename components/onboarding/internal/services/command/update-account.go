package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

const (
	accountTypeExternal = "external"
)

// UpdateAccount update an account from the repository by given id.
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, uai *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account")
	defer span.End()

	logger.Infof("Trying to update account: %v", uai)

	accFound, err := uc.findAndValidateAccount(ctx, organizationID, ledgerID, id, &span, logger)
	if err != nil {
		return nil, err
	}

	account := &mmodel.Account{
		Name:        uai.Name,
		Status:      uai.Status,
		EntityID:    uai.EntityID,
		SegmentID:   uai.SegmentID,
		PortfolioID: uai.PortfolioID,
		Metadata:    uai.Metadata,
		Blocked:     uai.Blocked,
	}

	accountUpdated, err := uc.updateAccountInRepo(ctx, organizationID, ledgerID, portfolioID, id, account, &span, logger)
	if err != nil {
		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Account{}).Name(), id.String(), uai.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	accountUpdated.Metadata = metadataUpdated

	if uai.Metadata == nil {
		assertMetadataUnchanged(accountUpdated.Metadata, accFound.Metadata, id, organizationID, ledgerID)
	}

	return accountUpdated, nil
}

// findAndValidateAccount finds the account and validates it's not external.
func (uc *UseCase) findAndValidateAccount(ctx context.Context, organizationID, ledgerID, id uuid.UUID, span *trace.Span, logger libLog.Logger) (*mmodel.Account, error) {
	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		logger.Errorf("Error finding account by alias: %v", err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find account by alias", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	if accFound != nil {
		assert.That(accFound.ID == id.String(),
			"found account ID must match requested ID",
			"requestedID", id.String(),
			"foundID", accFound.ID)
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == accountTypeExternal {
		return nil, pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	return accFound, nil
}

// updateAccountInRepo updates the account in the repository and validates the result.
func (uc *UseCase) updateAccountInRepo(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, account *mmodel.Account, span *trace.Span, logger libLog.Logger) (*mmodel.Account, error) {
	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, portfolioID, id, account)
	if err != nil {
		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Warnf("Account ID not found: %s", id.String())

			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	assert.NotNil(accountUpdated, "repository Update must return non-nil account on success",
		"account_id", id, "organization_id", organizationID, "ledger_id", ledgerID)
	assert.That(accountUpdated.ID == id.String(), "account id mismatch after update",
		"expected_id", id.String(), "actual_id", accountUpdated.ID)
	assert.That(accountUpdated.OrganizationID == organizationID.String(), "account organization id mismatch after update",
		"expected_organization_id", organizationID.String(), "actual_organization_id", accountUpdated.OrganizationID)
	assert.That(accountUpdated.LedgerID == ledgerID.String(), "account ledger id mismatch after update",
		"expected_ledger_id", ledgerID.String(), "actual_ledger_id", accountUpdated.LedgerID)

	return accountUpdated, nil
}

// assertMetadataUnchanged validates that metadata remains unchanged when update metadata is nil.
func assertMetadataUnchanged(updated, original map[string]any, id, organizationID, ledgerID uuid.UUID) {
	normalizeNilMetadata := func(m map[string]any) map[string]any {
		if m == nil {
			return map[string]any{}
		}

		return m
	}

	assert.That(reflect.DeepEqual(normalizeNilMetadata(updated), normalizeNilMetadata(original)),
		"account metadata should remain unchanged when update metadata is nil",
		"account_id", id, "organization_id", organizationID, "ledger_id", ledgerID)
}
