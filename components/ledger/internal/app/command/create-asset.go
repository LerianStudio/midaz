package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpointers"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
)

// CreateAsset creates a new asset persists data in the repository.
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *mmodel.CreateAssetInput) (*mmodel.Asset, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_asset")
	defer span.End()

	logger.Infof("Trying to create asset: %v", cii)

	var status mmodel.Status
	if cii.Status.IsEmpty() || common.IsNilOrEmpty(&cii.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cii.Status
	}

	status.Description = cii.Status.Description

	if err := common.ValidateType(cii.Type); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate asset type", err)

		return nil, common.ValidateBusinessError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	if err := common.ValidateCode(cii.Code); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate asset code", err)

		return nil, common.ValidateBusinessError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	if cii.Type == "currency" {
		if err := common.ValidateCurrency(cii.Code); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to validate asset currency", err)

			return nil, common.ValidateBusinessError(err, reflect.TypeOf(mmodel.Asset{}).Name())
		}
	}

	_, err := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to find asset by name or code", err)

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
		mopentelemetry.HandleSpanError(&span, "Failed to create asset", err)

		logger.Errorf("Error creating asset: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Asset{}).Name(), inst.ID, cii.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create asset metadata", err)

		logger.Errorf("Error creating asset metadata: %v", err)

		return nil, err
	}

	inst.Metadata = metadata

	aAlias := "@external/" + cii.Code
	aStatusDescription := "Account external created by asset: " + cii.Code

	account, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve asset external account", err)

		logger.Errorf("Error retrieving asset external account: %v", err)

		return nil, err
	}

	if len(account) == 0 {
		logger.Infof("Creating external account for asset: %s", cii.Code)

		balanceValue := float64(0)

		aBalance := a.Balance{
			Available: &balanceValue,
			OnHold:    &balanceValue,
			Scale:     &balanceValue,
		}

		eAccount := &a.Account{
			ID:              common.GenerateUUIDv7().String(),
			AssetCode:       cii.Code,
			Alias:           &aAlias,
			Name:            "External " + cii.Code,
			Type:            "external",
			OrganizationID:  organizationID.String(),
			LedgerID:        ledgerID.String(),
			ParentAccountID: nil,
			ProductID:       nil,
			PortfolioID:     nil,
			EntityID:        nil,
			Balance:         aBalance,
			Status: a.Status{
				Code:        "external",
				Description: &aStatusDescription,
			},
			AllowSending:   mpointers.Bool(true),
			AllowReceiving: mpointers.Bool(true),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		_, err = uc.AccountRepo.Create(ctx, eAccount)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to create asset external account", err)

			logger.Errorf("Error creating asset external account: %v", err)

			return nil, err
		}

		logger.Infof("External account created for asset %s with alias %s", cii.Code, aAlias)
	}

	return inst, nil
}
