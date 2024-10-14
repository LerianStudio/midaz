package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"
	"reflect"

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
			return common.ValidateBusinessError(cn.OrganizationIDNotFoundBusinessError, reflect.TypeOf(o.Organization{}).Name())
		}

		return err
	}

	return nil
}
