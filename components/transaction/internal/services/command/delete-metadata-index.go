package command

import (
	"context"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// DeleteMetadataIndex removes a metadata index from a specific entity collection.
func (uc *UseCase) DeleteMetadataIndex(ctx context.Context, organizationID, ledgerID uuid.UUID, entityName, indexName string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_metadata_index")
	defer span.End()

	logger.Infof("Initializing the delete metadata index operation: entity=%s, index=%s", entityName, indexName)

	if !strings.HasPrefix(indexName, "metadata.") {
		err := pkg.ValidateBusinessError(constant.ErrMetadataIndexDeletionForbidden, "metadata_index")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid index name format", err)

		return err
	}

	err := uc.MetadataRepo.DeleteIndex(ctx, entityName, indexName)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete metadata index", err)

		logger.Errorf("Failed to delete metadata index: %v", err)

		return err
	}

	logger.Infof("Metadata index deleted successfully: %v", indexName)

	return nil
}
