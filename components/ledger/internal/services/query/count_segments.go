// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"

	libObs "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// CountSegments returns the number of segments for the specified organization and ledger.
func (uc *UseCase) CountSegments(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_segments")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Counting segments for organization %s and ledger %s", organizationID, ledgerID))

	count, err := uc.SegmentRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoSegmentsFound, constant.EntitySegment)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("No segments found for organization: %s", organizationID.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count segments on repo", err)

			return 0, err
		}

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error counting segments on repo: %v", err))

		libOpentelemetry.HandleSpanError(span, "Failed to count segments on repo", err)

		return 0, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Found %d segments for organization %s and ledger %s", count, organizationID, ledgerID))

	return count, nil
}
