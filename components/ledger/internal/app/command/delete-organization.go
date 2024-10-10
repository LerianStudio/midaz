package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"github.com/google/uuid"
)

// DeleteOrganizationByID fetch a new organization from the repository
func (uc *UseCase) DeleteOrganizationByID(ctx context.Context, id string) error {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Remove organization for id: %s", id)

	if err := uc.OrganizationRepo.Delete(ctx, uuid.MustParse(id)); err != nil {
		logger.Errorf("Error deleting organization on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Organization{}).Name(),
				Code:       "0038",
				Title:      "Organization ID Not Found",
				Message:    "The provided organization ID does not exist in our records. Please verify the organization ID and try again.",
				Err:        err,
			}
		}

		return err
	}

	return nil
}
