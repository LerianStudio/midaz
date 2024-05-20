package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	"github.com/google/uuid"
)

// CreateInstrument creates a new instrument persists data in the repository.
func (uc *UseCase) CreateInstrument(ctx context.Context, organizationID, ledgerID uuid.UUID, cii *i.CreateInstrumentInput) (*i.Instrument, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create instrument: %v", cii)

	var status i.Status
	if cii.Status.IsEmpty() {
		status = i.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cii.Status
	}

	if err := common.ValidateType(cii.Type); err != nil {
		return nil, err
	}

	if err := common.IsUpper(cii.Code); err != nil {
		return nil, err
	}

	if cii.Type == "currency" {
		if err := common.ValidateCurrency(cii.Code); err != nil {
			return nil, err
		}
	}

	_, err := uc.InstrumentRepo.FindByNameOrCode(ctx, organizationID, ledgerID, cii.Name, cii.Code)
	if err != nil {
		logger.Errorf("Error creating instrument: %v", err)
		return nil, err
	}

	instrument := &i.Instrument{
		Name:           cii.Name,
		Type:           cii.Type,
		Code:           cii.Code,
		Status:         status,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	inst, err := uc.InstrumentRepo.Create(ctx, instrument)
	if err != nil {
		logger.Errorf("Error creating instrument: %v", err)
		return nil, err
	}

	if cii.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, cii.Metadata); err != nil {
			return nil, err
		}

		meta := m.Metadata{
			EntityID:   inst.ID,
			EntityName: reflect.TypeOf(i.Instrument{}).Name(),
			Data:       cii.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(i.Instrument{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating instrument metadata: %v", err)
			return nil, err
		}

		inst.Metadata = cii.Metadata
	}

	return inst, nil
}
