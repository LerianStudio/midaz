package command

import (
	"context"
	"errors"
	"reflect"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
)

// UpdateLedgerByID update a ledger from the repository.
func (uc *UseCase) UpdateLedgerByID(ctx context.Context, organizationID, id string, uli *l.UpdateLedgerInput) (*l.Ledger, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update ledger: %v", uli)

	ledger := &l.Ledger{
		Name:           uli.Name,
		OrganizationID: organizationID,
		Status:         uli.Status,
	}

	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, uuid.MustParse(organizationID), uuid.MustParse(id), ledger)
	if err != nil {
		logger.Errorf("Error updating ledger on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(l.Ledger{}).Name())
		}

		return nil, err
	}

	if len(uli.Metadata) > 0 {
		if err := common.CheckMetadataKeyAndValueLength(100, uli.Metadata); err != nil {
			return nil, common.ValidateBusinessError(err, reflect.TypeOf(l.Ledger{}).Name())
		}

		if err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(l.Ledger{}).Name(), id, uli.Metadata); err != nil {
			return nil, err
		}

		ledgerUpdated.Metadata = uli.Metadata
	}

	return ledgerUpdated, nil
}
