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
func (uc *UseCase) UpdateLedgerByID(ctx context.Context, organizationID, id uuid.UUID, uli *l.UpdateLedgerInput) (*l.Ledger, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to update ledger: %v", uli)

	ledger := &l.Ledger{
		Name:           uli.Name,
		OrganizationID: organizationID.String(),
		Status:         uli.Status,
	}

	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, organizationID, id, ledger)
	if err != nil {
		logger.Errorf("Error updating ledger on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.ValidateBusinessError(cn.ErrLedgerIDNotFound, reflect.TypeOf(l.Ledger{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(l.Ledger{}).Name(), id.String(), uli.Metadata)
	if err != nil {
		return nil, err
	}

	ledgerUpdated.Metadata = metadataUpdated

	return ledgerUpdated, nil
}
