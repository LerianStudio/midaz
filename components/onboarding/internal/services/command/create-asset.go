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

// CreateAsset creates a new asset and persists it in the repository.
//
// Assets represent financial instruments such as currencies (USD, EUR), cryptocurrencies (BTC, ETH),
// commodities (Gold, Silver), or other tradable items tracked in the ledger. Each asset has a unique
// code and can be used to denominate account balances and transactions.
//
// This function performs a critical operation: for each new asset, it automatically creates a special
// "external" account. External accounts serve as boundaries for the ledger system, representing flows
// in/out of the system (e.g., deposits from external banks, withdrawals to external accounts).
//
// The function performs the following steps:
// 1. Validates message broker availability (required for account queue)
// 2. Validates and normalizes asset status
// 3. Validates asset type (currency, crypto, commodities, others)
// 4. Validates asset code format (uppercase, alphanumeric with at least one letter)
// 5. For currency type, validates against ISO-4217 standard
// 6. Checks name/code uniqueness within the ledger
// 7. Persists the asset to PostgreSQL
// 8. Stores custom metadata in MongoDB if provided
// 9. Creates an automatic external account for the asset (if it doesn't exist)
// 10. Publishes external account to transaction service queue
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning this asset
//   - ledgerID: The UUID of the ledger containing this asset
//   - cii: The asset creation input containing all required fields
//
// Returns:
//   - *mmodel.Asset: The created asset with generated ID and metadata
//   - error: Business validation or persistence errors
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_asset")
	defer span.End()

	logger.Infof("Trying to create asset: %v", cii)

	// Step 1: Verify message broker health before proceeding.
	// We need RabbitMQ to publish the external account creation event.
	if !uc.RabbitMQRepo.CheckRabbitMQHealth() {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Message Broker is unavailable", err)

		logger.Warnf("Message Broker is unavailable: %v", err)

		return nil, err
	}

	// Step 2: Determine asset status, defaulting to ACTIVE if not specified
	var status mmodel.Status
	if cii.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cii.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cii.Status
	}

	status.Description = cii.Status.Description

	// Step 3: Validate asset type is one of: currency, crypto, commodities, others
	if err := libCommons.ValidateType(cii.Type); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrInvalidType, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset type", err)

		return nil, err
	}

	// Step 4: Validate asset code format.
	// Code must be uppercase, alphanumeric with at least one letter (e.g., USD, BTC, GOLD)
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

	// Step 5: For currency assets, validate against ISO-4217 standard.
	// This ensures compliance with international currency code standards.
	if cii.Type == "currency" {
		if err := libCommons.ValidateCurrency(cii.Code); err != nil {
			err := pkg.ValidateBusinessError(constant.ErrCurrencyCodeStandardCompliance, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset currency", err)

			return nil, err
		}
	}

	// Step 6: Verify asset name and code uniqueness within the ledger.
	// Duplicate asset names or codes can lead to confusion in financial operations.
	_, err := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find asset by name or code", err)

		logger.Errorf("Error creating asset: %v", err)

		return nil, err
	}

	// Step 7: Construct the asset entity with validated fields and timestamps
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

	// Step 8: Persist the asset to PostgreSQL
	inst, err := uc.AssetRepo.Create(ctx, asset)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset", err)

		logger.Errorf("Error creating asset: %v", err)

		return nil, err
	}

	// Step 9: Store custom metadata in MongoDB if provided
	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), inst.ID, cii.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset metadata", err)

		logger.Errorf("Error creating asset metadata: %v", err)

		return nil, err
	}

	inst.Metadata = metadata

	// Step 10: Create an external account for this asset if it doesn't already exist.
	// External accounts are system boundaries that track inflows/outflows to/from external systems.
	// Format: "@external/{ASSET_CODE}" (e.g., "@external/USD")
	aAlias := constant.DefaultExternalAccountAliasPrefix + cii.Code
	aStatusDescription := "Account external created by asset: " + cii.Code

	// Check if external account already exists for this asset
	account, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve asset external account", err)

		logger.Errorf("Error retrieving asset external account: %v", err)

		return nil, err
	}

	// Step 11: If external account doesn't exist, create it automatically.
	// This external account enables tracking of asset flows in/out of the ledger system.
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

		// Step 12: Publish external account creation to transaction service via RabbitMQ.
		// This allows the transaction service to initialize balance tracking.
		logger.Infof("Sending external account to transaction queue...")
		uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, *acc)
	}

	return inst, nil
}
