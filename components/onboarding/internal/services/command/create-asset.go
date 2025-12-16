package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// validateAssetInput validates the asset type and code
func (uc *UseCase) validateAssetInput(ctx context.Context, cii *mmodel.CreateAssetInput, span *trace.Span) error {
	if err := libCommons.ValidateType(cii.Type); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrInvalidType, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset type", err)

		return fmt.Errorf("validation failed: %w", err)
	}

	if err := uc.validateAssetCode(ctx, cii.Code); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if cii.Type == "currency" {
		if err := libCommons.ValidateCurrency(cii.Code); err != nil {
			err := pkg.ValidateBusinessError(constant.ErrCurrencyCodeStandardCompliance, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate asset currency", err)

			return fmt.Errorf("validation failed: %w", err)
		}
	}

	return nil
}

// createExternalAccountForAsset creates an external account and its default balance for an asset
func (uc *UseCase) createExternalAccountForAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput, requestID, token string, span *trace.Span) error {
	logger, _, _, spanCtx := libCommons.NewTrackingFromContext(ctx)
	_ = spanCtx // spanCtx intentionally unused

	aAlias := constant.DefaultExternalAccountAliasPrefix + cii.Code
	aStatusDescription := "Account external created by asset: " + cii.Code

	account, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve asset external account", err)

		logger.Errorf("Error retrieving asset external account: %v", err)

		return fmt.Errorf("operation failed: %w", err)
	}

	if len(account) > 0 {
		return nil
	}

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

	assert.That(assert.ValidUUID(eAccount.ID),
		"generated external account ID must be valid UUID",
		"asset_code", cii.Code)

	acc, err := uc.AccountRepo.Create(ctx, eAccount)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset external account", err)

		logger.Errorf("Error creating asset external account: %v", err)

		return fmt.Errorf("failed to create: %w", err)
	}

	assert.NotNil(acc, "repository Create must return non-nil external account on success",
		"asset_code", cii.Code)

	logger.Infof("External account created for asset %s with alias %s", cii.Code, aAlias)

	balanceInput := mmodel.CreateBalanceInput{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      uuid.MustParse(acc.ID),
		Alias:          aAlias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      cii.Code,
		AccountType:    "external",
		AllowSending:   true,
		AllowReceiving: true,
	}

	_, err = uc.BalancePort.CreateBalanceSync(ctx, balanceInput)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create default balance", err)

		logger.Errorf("Failed to create default balance: %v", err)

		var (
			unauthorized pkg.UnauthorizedError
			forbidden    pkg.ForbiddenError
		)

		if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
			return fmt.Errorf("operation failed: %w", err)
		}

		return fmt.Errorf("validation failed: %w", pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, reflect.TypeOf(mmodel.Account{}).Name()))
	}

	logger.Infof("External account default balance created via gRPC")

	return nil
}

// CreateAssetSync creates an asset and metadata synchronously and ensures an external
// account exists for the asset. If a new external account is created, it also
// creates the default balance for that account via gRPC.
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

	if err := uc.validateAssetInput(ctx, cii, &span); err != nil {
		return nil, err
	}

	_, err := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find asset by name or code", err)

		logger.Errorf("Error finding existing asset by code: %v", err)

		return nil, fmt.Errorf("failed to find: %w", err)
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

		return nil, fmt.Errorf("failed to create: %w", err)
	}

	assert.NotNil(inst, "repository Create must return non-nil asset on success",
		"asset_code", asset.Code)

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), inst.ID, cii.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset metadata", err)

		logger.Errorf("Error creating asset metadata: %v", err)

		return nil, fmt.Errorf("failed to create: %w", err)
	}

	inst.Metadata = metadata

	if err := uc.createExternalAccountForAsset(ctx, organizationID, ledgerID, cii, requestID, token, &span); err != nil {
		return nil, err
	}

	return inst, nil
}

// validateAssetCode checks the provided asset code and maps validation errors to business errors.
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

			return fmt.Errorf("invalid asset code format: %w", mapped)
		case constant.ErrCodeUppercaseRequirement.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrCodeUppercaseRequirement, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return fmt.Errorf("asset code uppercase requirement: %w", mapped)
		}
	}

	return nil
}
