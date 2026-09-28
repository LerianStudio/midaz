// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func (uc *UseCase) DeleteMetadataIndex(ctx context.Context, entityName, indexName string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_metadata_index")
	defer span.End()

	if !strings.HasPrefix(indexName, "metadata.") {
		err := pkg.ValidateBusinessError(constant.ErrMetadataIndexDeletionForbidden, "metadata_index")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid index name format", err)

		return err
	}

	err := uc.TransactionMetadataRepo.DeleteIndex(ctx, entityName, indexName)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to delete metadata index", err)

		return err
	}

	return nil
}
