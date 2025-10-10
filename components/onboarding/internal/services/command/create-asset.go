// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for creating a new asset.
package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateAsset creates a new asset and its associated external account.
//
// This use case is responsible for:
//  1. Validating the asset's type and code, including ISO 4217 compliance for currencies.
//  2. Ensuring the asset name and code are unique within the ledger.
//  3. Persisting the asset in PostgreSQL and its metadata in MongoDB.
//  4. Automatically creating a corresponding "external" account for the asset,
//     which is used for transactions involving external systems.
//  5. Publishing an event to RabbitMQ to initialize the external account's balance.
//
// Business Rules:
//   - Asset codes must be uppercase, alphanumeric, and contain at least one letter.
//   - Assets of type "currency" must have a code that complies with the ISO 4217 standard.
//   - An external account is created for each new asset with the alias "@external/{CODE}".
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization that owns the asset.
//   - ledgerID: The UUID of the ledger where the asset will be created.
//   - cii: The input data for creating the asset.
//
// Returns:
//   - *mmodel.Asset: The newly created asset, complete with its metadata.
//   - error: An error if the creation fails due to business rule violations or database issues.
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_asset")
	defer span.End()

	logger.Infof("Trying to create asset: %v", cii)

	if !uc.RabbitMQRepo.CheckRabbitMQHealth() {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Message Broker is unavailable", err)

		logger.Warnf("Message Broker is unavailable: %v", err)

		return nil, err
	}

	var status mmodel.Status
	if cii.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cii.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cii.Status
	}

	status.Description = cii.Status.Description

	if err := libCommons.ValidateType(cii.Type); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrInvalidType, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset type", err)

		return nil, err
	}

	if err := libCommons.ValidateCode(cii.Code); err != nil {
		// FIXME: This error handling is repetitive. Refactor into a helper function
		// to reduce code duplication and improve readability.
		if err.Error() == constant.ErrInvalidCodeFormat.Error() {
			err := pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", err)

			return nil, err
		} else if err.Error() == constant.ErrCodeUppercaseRequirement.Error() {
			err := pkg.ValidateBusinessError(constant.ErrCodeUppercaseRequirement, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", err)

			return nil, err
		}
	}

	if cii.Type == "currency" {
		if err := libCommons.ValidateCurrency(cii.Code); err != nil {
			err := pkg.ValidateBusinessError(constant.ErrCurrencyCodeStandardCompliance, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset currency", err)

			return nil, err
		}
	}

	// FIXME: This logic is incorrect. FindByNameOrCode returns an error if the asset is *not* found.
	// The code should check if the error is `services.ErrDatabaseItemNotFound` and proceed in that case.
	// If the error is nil, it means an asset with the same name or code already exists, and an
	// `ErrAssetNameOrCodeDuplicate` error should be returned. Any other error should be returned directly.
	_, err := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find asset by name or code", err)

		logger.Errorf("Error creating asset: %v", err)

		return nil, err
	}

	asset := &mmodel.Asset{
		Name:           cii.Name,
		Type:           cii.Type,
		Code:           cii.Code,
		Status:         status,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	inst, err := uc.AssetRepo.Create(ctx, asset)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset", err)

		logger.Errorf("Error creating asset: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), inst.ID, cii.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset metadata", err)

		logger.Errorf("Error creating asset metadata: %v", err)

		return nil, err
	}

	inst.Metadata = metadata

	aAlias := constant.DefaultExternalAccountAliasPrefix + cii.Code
	aStatusDescription := "Account external created by asset: " + cii.Code

	account, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve asset external account", err)

		logger.Errorf("Error retrieving asset external account: %v", err)

		return nil, err
	}

	if len(account) == 0 {
		logger.Infof("Creating external account for asset: %s", cii.Code)

		eAccount := &mmodel.Account{
			ID:              libCommons.GenerateUUIDv7().String(),
			AssetCode:       cii.Code,
			Alias:           &aAlias,
			Name:            "External " + cii.Code,
			Type:            "external",
			OrganizationID:  organizationID.String(),
			LedgerID:        ledgerID.String(),
			ParentAccountID: nil,
			SegmentID:       nil,
			PortfolioID:     nil,
			EntityID:        nil,
			Status: mmodel.Status{
				Code:        "external",
				Description: &aStatusDescription,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		acc, err := uc.AccountRepo.Create(ctx, eAccount)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset external account", err)

			logger.Errorf("Error creating asset external account: %v", err)

			return nil, err
		}

		logger.Infof("External account created for asset %s with alias %s", cii.Code, aAlias)

		logger.Infof("Sending external account to transaction queue...")
		uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, *acc)
	}

	return inst, nil
}
