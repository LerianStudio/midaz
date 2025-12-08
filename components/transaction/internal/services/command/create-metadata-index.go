package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateMetadataIndex creates a new metadata index.
func (uc *UseCase) CreateMetadataIndex(ctx context.Context, organizationID, ledgerID uuid.UUID, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_metadata_index")
	defer span.End()

	logger.Infof("Initializing the create metadata index operation: %v", input)

	return nil, nil
}
