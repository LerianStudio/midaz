package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	grpcMetadata "google.golang.org/grpc/metadata"
)

// CreateAsset creates an asset and metadata synchronously and ensures an external
// account exists for the asset. If a new external account is created, it also
// creates the default balance for that account.
// The balance is created via the BalancePort interface, which can be either local (in-process)
// or remote (gRPC) depending on the deployment mode.
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput, token string) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

	if err := utils.ValidateType(cii.Type); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrInvalidType, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset type", err)

		return nil, err
	}

	if err := uc.validateAssetCode(ctx, cii.Code); err != nil {
		return nil, err
	}

	if cii.Type == "currency" {
		if err := utils.ValidateCurrency(cii.Code); err != nil {
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

		// Inject authorization token into context metadata for downstream gRPC calls
		ctxWithAuth := grpcMetadata.AppendToOutgoingContext(ctx, libConstant.MetadataAuthorization, token)

		_, err = uc.BalancePort.CreateBalanceSync(ctxWithAuth, balanceInput)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create default balance", err)

			logger.Errorf("Failed to create default balance: %v", err)

			var (
				unauthorized pkg.UnauthorizedError
				forbidden    pkg.ForbiddenError
			)

			if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
				return nil, err
			}

			return nil, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, reflect.TypeOf(mmodel.Account{}).Name())
		}

		logger.Infof("External account default balance created")
	}

	return inst, nil
}

// validateAssetCode checks the provided asset code and maps validation errors to business errors.
func (uc *UseCase) validateAssetCode(ctx context.Context, code string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "command.validate_asset_code")
	defer span.End()

	logger.Infof("Validating asset code: %s", code)

	if err := utils.ValidateCode(code); err != nil {
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
