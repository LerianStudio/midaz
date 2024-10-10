package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
)

// DeleteLedgerByID deletes a ledger from the repository
func (uc *UseCase) DeleteLedgerByID(ctx context.Context, organizationID, id string) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove ledger for id: %s", id)

	if err := uc.LedgerRepo.Delete(ctx, uuid.MustParse(organizationID), uuid.MustParse(id)); err != nil {
		logger.Errorf("Error deleting ledger on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.EntityNotFoundError{
				EntityType: reflect.TypeOf(l.Ledger{}).Name(),
				Code:       "0037",
				Title:      "Ledger ID Not Found",
				Message:    "The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.",
				Err:        err,
			}
		}

		return err
	}

	return nil
}
