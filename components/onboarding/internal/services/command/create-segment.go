package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"reflect"
	"time"
)

// CreateSegment creates a new segment persists data in the repository.
func (uc *UseCase) CreateSegment(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_segment")
	defer span.End()

	logger.Infof("Trying to create segment: %v", cpi)

	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	segment := &mmodel.Segment{
		ID:             libCommons.GenerateUUIDv7().String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	_, err := uc.SegmentRepo.FindByName(ctx, organizationID, ledgerID, cpi.Name)
	if err != nil {
		libCommons.NewLoggerFromContext(ctx).Errorf("Error finding segment by name: %v", err)

		return nil, err
	}

	prod, err := uc.SegmentRepo.Create(ctx, segment)
	if err != nil {
		libCommons.NewLoggerFromContext(ctx).Errorf("Error creating segment: %v", err)

		logger.Errorf("Error creating segment: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), prod.ID, cpi.Metadata)
	if err != nil {
		libCommons.NewLoggerFromContext(ctx).Errorf("Error creating segment metadata: %v", err)

		logger.Errorf("Error creating segment metadata: %v", err)

		return nil, err
	}

	prod.Metadata = metadata

	return prod, nil
}
