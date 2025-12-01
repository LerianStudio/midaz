package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	balanceproto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateAsset creates an asset and its associated external account synchronously.
//
// This method creates a new asset within a ledger and automatically provisions
// the corresponding external account and default balance. The external account
// serves as the counterparty for all external transactions involving this asset.
//
// Creation Process:
//
//	Step 1: Context Setup
//	  - Extract logger, tracer, and requestID from context
//	  - Start OpenTelemetry span "command.create_asset"
//
//	Step 2: Status Resolution
//	  - If input status is empty or has no code: Default to "ACTIVE"
//	  - Otherwise: Use provided status code
//	  - Apply status description from input (optional)
//
//	Step 3: Asset Type Validation
//	  - Validate asset type via libCommons.ValidateType
//	  - Supported types: "currency", "commodity", "crypto", etc.
//	  - If invalid type: Return ErrInvalidType business error
//
//	Step 4: Asset Code Validation
//	  - Validate code format via validateAssetCode (uppercase, alphanumeric)
//	  - If format invalid: Return ErrInvalidCodeFormat or ErrCodeUppercaseRequirement
//	  - For type="currency": Additional ISO 4217 validation
//	  - If currency code invalid: Return ErrCurrencyCodeStandardCompliance
//
//	Step 5: Uniqueness Validation
//	  - Check if asset with same name OR code exists in ledger
//	  - If duplicate found: Return error from repository
//
//	Step 6: Asset Persistence
//	  - Create asset model with validated properties
//	  - Persist to PostgreSQL via AssetRepo.Create
//	  - If creation fails: Return error with span event
//
//	Step 7: Metadata Creation
//	  - Create metadata document in MongoDB via CreateMetadata
//	  - If metadata creation fails: Return error (asset already created)
//
//	Step 8: External Account Provisioning
//	  - Check if external account for asset code already exists
//	  - If not exists: Create external account with alias "@external/{CODE}"
//	  - External account has type="external", status="external"
//
//	Step 9: Default Balance Creation (gRPC)
//	  - For new external accounts: Create default balance via gRPC
//	  - Balance has default key, allows sending and receiving
//	  - If gRPC fails with auth error: Return auth error as-is
//	  - If other gRPC error: Return ErrAccountCreationFailed
//
//	Step 10: Response Assembly
//	  - Attach metadata to asset entity
//	  - Return complete asset with generated ID
//
// External Account Purpose:
//
// Every asset requires an external account to serve as the counterparty for
// transactions that move value in/out of the ledger. For example, when a
// customer deposits USD, the transaction debits the external USD account
// and credits the customer's account.
//
// Asset Types:
//
//   - "currency": Fiat currencies (ISO 4217 validation required)
//   - "commodity": Physical commodities
//   - "crypto": Cryptocurrencies
//   - Custom types as defined by the organization
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger to contain the asset
//   - cii: Creation input with name, type, code, optional status, and metadata
//   - token: Authentication token for gRPC balance service call
//
// Returns:
//   - *mmodel.Asset: Created asset with generated ID and metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrInvalidType: Asset type not recognized
//   - ErrInvalidCodeFormat: Code format validation failed
//   - ErrCodeUppercaseRequirement: Code must be uppercase
//   - ErrCurrencyCodeStandardCompliance: Currency code not ISO 4217 compliant
//   - Asset name or code already exists in ledger
//   - Database connection failure
//   - gRPC balance creation failure
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput, token string) (*mmodel.Asset, error) {
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_asset")
	defer span.End()

	logger.Infof("Trying to create asset (sync): %v", cii)

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

	if err := uc.validateAssetCode(ctx, cii.Code); err != nil {
		return nil, err
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

		balanceReq := &balanceproto.BalanceRequest{
			RequestId:      requestID,
			OrganizationId: organizationID.String(),
			LedgerId:       ledgerID.String(),
			AccountId:      acc.ID,
			Alias:          aAlias,
			Key:            constant.DefaultBalanceKey,
			AssetCode:      cii.Code,
			AccountType:    "external",
			AllowSending:   true,
			AllowReceiving: true,
		}

		_, err = uc.BalanceGRPCRepo.CreateBalance(ctx, token, balanceReq)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create default balance via gRPC", err)

			logger.Errorf("Failed to create default balance via gRPC: %v", err)

			var (
				unauthorized pkg.UnauthorizedError
				forbidden    pkg.ForbiddenError
			)

			if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
				return nil, err
			}

			return nil, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, reflect.TypeOf(mmodel.Account{}).Name())
		}

		logger.Infof("External account default balance created via gRPC")
	}

	return inst, nil
}

// validateAssetCode validates asset code format and maps errors to business errors.
//
// Asset codes must follow specific formatting rules:
//   - Alphanumeric characters only
//   - Uppercase letters required
//   - No special characters or spaces
//
// Validation Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "command.validate_asset_code"
//
//	Step 2: Code Validation
//	  - Call libCommons.ValidateCode with asset code
//	  - Map validation errors to domain-specific business errors
//
//	Step 3: Error Mapping
//	  - ErrInvalidCodeFormat: Code contains invalid characters
//	  - ErrCodeUppercaseRequirement: Code must be uppercase
//
// Parameters:
//   - ctx: Request context with tracing information
//   - code: Asset code string to validate
//
// Returns:
//   - error: Mapped business error or nil if valid
func (uc *UseCase) validateAssetCode(ctx context.Context, code string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "command.validate_asset_code")
	defer span.End()

	logger.Infof("Validating asset code: %s", code)

	if err := libCommons.ValidateCode(code); err != nil {
		switch err.Error() {
		case constant.ErrInvalidCodeFormat.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return mapped
		case constant.ErrCodeUppercaseRequirement.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrCodeUppercaseRequirement, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return mapped
		}
	}

	return nil
}
