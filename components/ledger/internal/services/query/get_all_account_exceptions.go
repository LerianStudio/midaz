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
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// GetAllAccountExceptions returns one page of live account exceptions for an account.
//
// Pagination is page-based (limit/offset), the frozen contract for this listing. An empty
// page is a business condition, not a technical one: the repository returns an empty slice
// and this layer raises 0504 (ErrNoAccountExceptionsFound), following the repo-wide
// empty-list convention.
func (uc *UseCase) GetAllAccountExceptions(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*mmodel.AccountException, http.Pagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_account_exceptions")
	defer span.End()

	pagination := http.Pagination{
		Limit: filter.Limit,
		Page:  filter.Page,
	}

	exceptions, err := uc.AccountExceptionRepo.FindAllByAccountID(ctx, organizationID, ledgerID, accountID, pagination)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get account exceptions on repo", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get account exceptions on repo", libLog.Err(err))

		return nil, http.Pagination{}, err
	}

	if len(exceptions) == 0 {
		err := pkg.ValidateBusinessError(constant.ErrNoAccountExceptionsFound, constant.EntityAccountException)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No account exceptions found", err)
		logger.Log(ctx, libLog.LevelWarn, "No account exceptions found",
			libLog.String("account_id", accountID.String()))

		return nil, http.Pagination{}, err
	}

	pagination.SetItems(exceptions)

	return exceptions, pagination, nil
}
