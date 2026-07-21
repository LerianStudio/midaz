// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
)

// CountAccountsByHolderID returns the number of active accounts owned by the
// holder within the organization, across all ledgers. A zero count is a valid
// result (the holder owns no accounts); only infrastructure failures surface as
// errors.
func (uc *UseCase) CountAccountsByHolderID(ctx context.Context, organizationID, holderID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_accounts_by_holder")
	defer span.End()

	count, err := uc.AccountRepo.CountByHolderID(ctx, organizationID, holderID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error counting accounts by holder on repo", libLog.Err(err))

		libOpentelemetry.HandleSpanError(span, "Failed to count accounts by holder on repo", err)

		return 0, err
	}

	return count, nil
}
