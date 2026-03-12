// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"reflect"

	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// DeleteSegmentByID delete a segment from the repository by ids.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) DeleteSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_segment_by_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Remove segment for id: %s", id.String()))

	if err := uc.SegmentRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Segment ID not found: %s", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete segment on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete segment on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error deleting segment: %v", err))

		return err
	}

	return nil
}
