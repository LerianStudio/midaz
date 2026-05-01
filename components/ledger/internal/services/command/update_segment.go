// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateSegmentByID updates a segment from the repository by the given ID.
func (uc *UseCase) UpdateSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_segment_by_id")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if upi.Name != "" {
		segmentFound, err := uc.SegmentRepo.Find(ctx, organizationID, ledgerID, id)
		if err != nil {
			if errors.Is(err, services.ErrDatabaseItemNotFound) {
				err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, constant.EntitySegment)

				logger.Log(ctx, libLog.LevelWarn, "Segment not found", libLog.Err(err), libLog.String("segment_id", id.String()))
			} else {
				logger.Log(ctx, libLog.LevelError, "Failed to find segment", libLog.Err(err), libLog.String("segment_id", id.String()))
			}

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find segment on repo by id", err)

			return nil, err
		}

		if segmentFound != nil && segmentFound.Name != upi.Name {
			if _, err := uc.SegmentRepo.ExistsByName(ctx, organizationID, ledgerID, upi.Name); err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check segment name existence", err)
				logger.Log(ctx, libLog.LevelWarn, "Segment name is not available", libLog.Err(err), libLog.String("segment_id", id.String()))

				return nil, err
			}
		}
	}

	segment := &mmodel.Segment{
		Name:   upi.Name,
		Status: upi.Status,
	}

	segmentUpdated, err := uc.SegmentRepo.Update(ctx, organizationID, ledgerID, id, segment)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, constant.EntitySegment)
			logger.Log(ctx, libLog.LevelWarn, "Segment not found", libLog.Err(err), libLog.String("segment_id", id.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update segment on repo by id", err)

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to update segment", libLog.Err(err), libLog.String("segment_id", id.String()))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update segment on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntitySegment, id.String(), upi.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to update segment metadata", libLog.Err(err), libLog.String("segment_id", id.String()))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	segmentUpdated.Metadata = metadataUpdated

	return segmentUpdated, nil
}
