// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateSegment creates a new segment and persists it in the repository.
func (uc *UseCase) CreateSegment(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_segment")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to create segment organizationID=%s ledgerID=%s name=%s", organizationID.String(), ledgerID.String(), cpi.Name))

	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	segmentID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate segment ID", err)
		logger.Log(ctx, libLog.LevelError, "Error generating segment ID")

		return nil, err
	}

	segment := &mmodel.Segment{
		ID:             segmentID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	_, err = uc.SegmentRepo.FindByName(ctx, organizationID, ledgerID, cpi.Name)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find segment by name", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error finding segment by name: %v", err))

		return nil, err
	}

	prod, err := uc.SegmentRepo.Create(ctx, segment)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create segment", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating segment: %v", err))

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), prod.ID, cpi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create segment metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating segment metadata: %v", err))

		return nil, err
	}

	prod.Metadata = metadata

	return prod, nil
}
