// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateSegmentByID update a segment from the repository by given id.
func (uc *UseCase) UpdateSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_segment_by_id")
	defer span.End()

	logger.Infof("Trying to update segment: %v", upi)

	segment := &mmodel.Segment{
		Name:   upi.Name,
		Status: upi.Status,
	}

	segmentUpdated, err := uc.SegmentRepo.Update(ctx, organizationID, ledgerID, id, segment)
	if err != nil {
		logger.Errorf("Error updating segment on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())

			logger.Warnf("Segment ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update segment on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update segment on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	segmentUpdated.Metadata = metadataUpdated

	return segmentUpdated, nil
}
