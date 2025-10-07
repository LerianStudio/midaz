// Package command implements write operations (commands) for the onboarding service.
// This file contains the CreateAsset command implementation.
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

// CreateAsset creates a new asset and automatically creates an associated external account.
//
// This method implements the create asset use case, which:
// 1. Validates RabbitMQ health (required for sending external account to transaction service)
// 2. Sets default status to ACTIVE if not provided
// 3. Validates asset type (currency, crypto, commodities, others)
// 4. Validates asset code format (alphanumeric, uppercase, at least one letter)
// 5. Validates currency code compliance (ISO 4217) if type is "currency"
// 6. Checks for duplicate asset name or code
// 7. Creates the asset in PostgreSQL
// 8. Creates associated metadata in MongoDB
// 9. Creates an external account for the asset (if not already exists)
// 10. Sends external account to transaction service queue
// 11. Returns the complete asset with metadata
//
// Business Rules:
//   - Asset codes must be uppercase and alphanumeric with at least one letter
//   - Currency type assets must comply with ISO 4217 standard
//   - Asset name and code must be unique within a ledger
//   - Status defaults to ACTIVE if not provided
//   - External account is automatically created with alias "@external/{CODE}"
//   - External account has type "external" and status "external"
//   - RabbitMQ must be healthy (external accounts need to be sent to transaction service)
//
// External Account:
//   - Automatically created for each asset
//   - Alias: "@external/{ASSET_CODE}" (e.g., "@external/USD")
//   - Name: "External {ASSET_CODE}" (e.g., "External USD")
//   - Type: "external"
//   - Used for transactions with external systems
//   - Only created once (checked before creation)
//
// Data Storage:
//   - Primary data: PostgreSQL (assets table, accounts table)
//   - Metadata: MongoDB (flexible key-value storage)
//   - Queue: RabbitMQ (external account creation event)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization that owns this asset
//   - ledgerID: UUID of the ledger that contains this asset
//   - cii: Create asset input with name, type, code, status, and metadata
//
// Returns:
//   - *mmodel.Asset: Created asset with metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrMessageBrokerUnavailable: RabbitMQ is not healthy
//   - ErrInvalidType: Asset type is not valid
//   - ErrInvalidCodeFormat: Code format is invalid
//   - ErrCodeUppercaseRequirement: Code is not uppercase
//   - ErrCurrencyCodeStandardCompliance: Currency code doesn't comply with ISO 4217
//   - ErrAssetNameOrCodeDuplicate: Asset with same name or code already exists
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.CreateAssetInput{
//	    Name: "US Dollar",
//	    Type: "currency",
//	    Code: "USD",
//	    Status: mmodel.Status{Code: "ACTIVE"},
//	}
//	asset, err := useCase.CreateAsset(ctx, orgID, ledgerID, input)
//	if err != nil {
//	    return nil, err
//	}
//	// External account "@external/USD" is automatically created
//
// OpenTelemetry:
//   - Creates span "command.create_asset"
//   - Records errors as span events
//   - Tracks validation and creation steps
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
