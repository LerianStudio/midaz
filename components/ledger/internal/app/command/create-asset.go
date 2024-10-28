package command

import (
	"context"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/google/uuid"
)

// CreateAsset creates a new asset persists data in the repository.
func (uc *UseCase) CreateAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *s.CreateAssetInput) (*s.Asset, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create asset: %v", cii)

	var status s.Status
	if cii.Status.IsEmpty() {
		status = s.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cii.Status
	}

	if err := common.ValidateType(cii.Type); err != nil {
		return nil, common.ValidateBusinessError(err, reflect.TypeOf(s.Asset{}).Name())
	}

	if err := common.ValidateCode(cii.Code); err != nil {
		return nil, common.ValidateBusinessError(err, reflect.TypeOf(s.Asset{}).Name())
	}

	if cii.Type == "currency" {
		if err := common.ValidateCurrency(cii.Code); err != nil {
			return nil, common.ValidateBusinessError(err, reflect.TypeOf(s.Asset{}).Name())
		}
	}

	_, err := uc.AssetRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		logger.Errorf("Error creating asset: %v", err)
		return nil, err
	}

	asset := &s.Asset{
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
		logger.Errorf("Error creating asset: %v", err)
		return nil, err
	}

	if cii.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, cii.Metadata); err != nil {
			return nil, common.ValidateBusinessError(err, reflect.TypeOf(s.Asset{}).Name())
		}

		meta := m.Metadata{
			EntityID:   inst.ID,
			EntityName: reflect.TypeOf(s.Asset{}).Name(),
			Data:       cii.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(s.Asset{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating asset metadata: %v", err)
			return nil, err
		}

		inst.Metadata = cii.Metadata
	}

	aAlias := "@external/" + cii.Code
	aStatusDescription := "Account external created by asset: " + cii.Code

	account, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
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
				Code:           "external",
				Description:    &aStatusDescription,
				AllowSending:   true,
				AllowReceiving: true,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		_, err = uc.AccountRepo.Create(ctx, eAccount)
		if err != nil {
			logger.Errorf("Error creating asset external account: %v", err)
			return nil, err
		}

		logger.Infof("External account created for asset %s with alias %s", cii.Code, aAlias)
	}

	return inst, nil
}
