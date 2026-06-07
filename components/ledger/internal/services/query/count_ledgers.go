// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
)

// CountLedgers returns the total count of ledgers for a specific organization.
func (uc *UseCase) CountLedgers(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_ledgers")
	defer span.End()

	count, err := uc.LedgerRepo.Count(ctx, organizationID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error counting ledgers on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoLedgersFound, constant.EntityLedger)

			logger.Log(ctx, libLog.LevelWarn, "No ledgers found for organization")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count ledgers on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count ledgers on repo", err)

		return 0, err
	}

	return count, nil
}
