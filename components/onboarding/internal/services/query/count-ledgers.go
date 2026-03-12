// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// CountLedgers returns the total count of ledgers for a specific organization
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) CountLedgers(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_ledgers")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Counting ledgers for organization: %s", organizationID))

	count, err := uc.LedgerRepo.Count(ctx, organizationID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error counting ledgers on repo: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("No ledgers found for organization: %s", organizationID.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count ledgers on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count ledgers on repo", err)

		return 0, err
	}

	return count, nil
}
