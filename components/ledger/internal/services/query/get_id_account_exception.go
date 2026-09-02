// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// GetAccountExceptionByID retrieves a single live account exception by scoped ID.
//
// The repository returns the 0503 business error directly when no live row matches, so
// this layer records it by class and returns it untouched — no remapping is needed.
func (uc *UseCase) GetAccountExceptionByID(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*mmodel.AccountException, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_exception_by_id")
	defer span.End()

	exception, err := uc.AccountExceptionRepo.FindByID(ctx, organizationID, ledgerID, accountID, id)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account exception by id", err)
			logger.Log(ctx, libLog.LevelWarn, "Account exception not found by id",
				libLog.String("account_exception_id", id.String()), libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get account exception by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get account exception by id", libLog.Err(err))

		return nil, err
	}

	return exception, nil
}
